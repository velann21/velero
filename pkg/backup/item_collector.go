/*
Copyright 2017, 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package backup

import (
	"encoding/json"
	"io/ioutil"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/velann21/velero/pkg/client"
	"github.com/velann21/velero/pkg/discovery"
	"github.com/velann21/velero/pkg/kuberesource"
	"github.com/velann21/velero/pkg/util/collections"
)

// itemCollector collects items from the Kubernetes API according to
// the backup spec and writes them to files inside dir.
type itemCollector struct {
	log                   logrus.FieldLogger
	backupRequest         *Request
	discoveryHelper       discovery.Helper
	dynamicFactory        client.DynamicFactory
	cohabitatingResources map[string]*cohabitatingResource
	dir                   string
}

type kubernetesResource struct {
	groupResource         schema.GroupResource
	preferredGVR          schema.GroupVersionResource
	namespace, name, path string
}

// getAllItems gets all relevant items from all API groups.
func (r *itemCollector) getAllItems() []*kubernetesResource {
	var resources []*kubernetesResource
	for _, group := range r.discoveryHelper.Resources() {
		groupItems, err := r.getGroupItems(r.log, group)
		if err != nil {
			r.log.WithError(err).WithField("apiGroup", group.String()).Error("Error collecting resources from API group")
			continue
		}

		resources = append(resources, groupItems...)
	}

	return resources
}

// getGroupItems collects all relevant items from a single API group.
func (r *itemCollector) getGroupItems(log logrus.FieldLogger, group *metav1.APIResourceList) ([]*kubernetesResource, error) {
	log = log.WithField("group", group.GroupVersion)

	log.Infof("Getting items for group")

	// Parse so we can check if this is the core group
	gv, err := schema.ParseGroupVersion(group.GroupVersion)
	if err != nil {
		return nil, errors.Wrapf(err, "error parsing GroupVersion %q", group.GroupVersion)
	}
	if gv.Group == "" {
		// This is the core group, so make sure we process in the following order: pods, pvcs, pvs,
		// everything else.
		sortCoreGroup(group)
	}

	var items []*kubernetesResource
	for _, resource := range group.APIResources {
		resourceItems, err := r.getResourceItems(log, gv, resource)
		if err != nil {
			log.WithError(err).WithField("resource", resource.String()).Error("Error getting items for resource")
			continue
		}

		items = append(items, resourceItems...)
	}

	return items, nil
}

// getResourceItems collects all relevant items for a given group-version-resource.
func (r *itemCollector) getResourceItems(log logrus.FieldLogger, gv schema.GroupVersion, resource metav1.APIResource) ([]*kubernetesResource, error) {
	log = log.WithField("resource", resource.Name)

	log.Info("Getting items for resource")

	var (
		gvr           = gv.WithResource(resource.Name)
		gr            = gvr.GroupResource()
		clusterScoped = !resource.Namespaced
	)

	// Getting the preferred group version of this resource
	preferredGVR, _, err := r.discoveryHelper.ResourceFor(gr.WithVersion(""))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// If the resource we are backing up is NOT namespaces, and it is cluster-scoped, check to see if
	// we should include it based on the IncludeClusterResources setting.
	if gr != kuberesource.Namespaces && clusterScoped {
		if r.backupRequest.Spec.IncludeClusterResources == nil {
			if !r.backupRequest.NamespaceIncludesExcludes.IncludeEverything() {
				// when IncludeClusterResources == nil (auto), only directly
				// back up cluster-scoped resources if we're doing a full-cluster
				// (all namespaces) backup. Note that in the case of a subset of
				// namespaces being backed up, some related cluster-scoped resources
				// may still be backed up if triggered by a custom action (e.g. PVC->PV).
				// If we're processing namespaces themselves, we will not skip here, they may be
				// filtered out later.
				log.Info("Skipping resource because it's cluster-scoped and only specific namespaces are included in the backup")
				return nil, nil
			}
		} else if !*r.backupRequest.Spec.IncludeClusterResources {
			log.Info("Skipping resource because it's cluster-scoped")
			return nil, nil
		}
	}

	if !r.backupRequest.ResourceIncludesExcludes.ShouldInclude(gr.String()) {
		log.Infof("Skipping resource because it's excluded")
		return nil, nil
	}

	if cohabitator, found := r.cohabitatingResources[resource.Name]; found {
		if cohabitator.seen {
			log.WithFields(
				logrus.Fields{
					"cohabitatingResource1": cohabitator.groupResource1.String(),
					"cohabitatingResource2": cohabitator.groupResource2.String(),
				},
			).Infof("Skipping resource because it cohabitates and we've already processed it")
			return nil, nil
		}
		cohabitator.seen = true
	}

	namespacesToList := getNamespacesToList(r.backupRequest.NamespaceIncludesExcludes)

	// Check if we're backing up namespaces, and only certain ones
	if gr == kuberesource.Namespaces && namespacesToList[0] != "" {
		resourceClient, err := r.dynamicFactory.ClientForGroupVersionResource(gv, resource, "")
		if err != nil {
			log.WithError(err).Error("Error getting dynamic client")
		} else {
			var labelSelector labels.Selector
			if r.backupRequest.Spec.LabelSelector != nil {
				labelSelector, err = metav1.LabelSelectorAsSelector(r.backupRequest.Spec.LabelSelector)
				if err != nil {
					// This should never happen...
					return nil, errors.Wrap(err, "invalid label selector")
				}
			}

			var items []*kubernetesResource
			for _, ns := range namespacesToList {
				log = log.WithField("namespace", ns)
				log.Info("Getting namespace")
				unstructured, err := resourceClient.Get(ns, metav1.GetOptions{})
				if err != nil {
					log.WithError(errors.WithStack(err)).Error("Error getting namespace")
					continue
				}

				labels := labels.Set(unstructured.GetLabels())
				if labelSelector != nil && !labelSelector.Matches(labels) {
					log.Info("Skipping namespace because it does not match the backup's label selector")
					continue
				}

				path, err := r.writeToFile(unstructured)
				if err != nil {
					log.WithError(err).Error("Error writing item to file")
					continue
				}

				items = append(items, &kubernetesResource{
					groupResource: gr,
					preferredGVR:  preferredGVR,
					name:          ns,
					path:          path,
				})
			}

			return items, nil
		}
	}

	// If we get here, we're backing up something other than namespaces
	if clusterScoped {
		namespacesToList = []string{""}
	}

	var items []*kubernetesResource

	for _, namespace := range namespacesToList {
		log = log.WithField("namespace", namespace)

		resourceClient, err := r.dynamicFactory.ClientForGroupVersionResource(gv, resource, namespace)
		if err != nil {
			log.WithError(err).Error("Error getting dynamic client")
			continue
		}

		var labelSelector string
		if selector := r.backupRequest.Spec.LabelSelector; selector != nil {
			labelSelector = metav1.FormatLabelSelector(selector)
		}

		log.Info("Listing items")
		unstructuredList, err := resourceClient.List(metav1.ListOptions{LabelSelector: labelSelector})
		if err != nil {
			log.WithError(errors.WithStack(err)).Error("Error listing items")
			continue
		}
		log.Infof("Retrieved %d items", len(unstructuredList.Items))

		// collect the items
		for i := range unstructuredList.Items {
			item := &unstructuredList.Items[i]

			if gr == kuberesource.Namespaces && !r.backupRequest.NamespaceIncludesExcludes.ShouldInclude(item.GetName()) {
				log.WithField("name", item.GetName()).Info("Skipping namespace because it's excluded")
				continue
			}

			path, err := r.writeToFile(item)
			if err != nil {
				log.WithError(err).Error("Error writing item to file")
				continue
			}

			items = append(items, &kubernetesResource{
				groupResource: gr,
				preferredGVR:  preferredGVR,
				namespace:     item.GetNamespace(),
				name:          item.GetName(),
				path:          path,
			})
		}
	}

	return items, nil
}

func (r *itemCollector) writeToFile(item *unstructured.Unstructured) (string, error) {
	f, err := ioutil.TempFile(r.dir, "")
	if err != nil {
		return "", errors.Wrap(err, "error creating temp file")
	}
	defer f.Close()

	jsonBytes, err := json.Marshal(item)
	if err != nil {
		return "", errors.Wrap(err, "error converting item to JSON")
	}

	if _, err := f.Write(jsonBytes); err != nil {
		return "", errors.Wrap(err, "error writing JSON to file")
	}

	if err := f.Close(); err != nil {
		return "", errors.Wrap(err, "error closing file")
	}

	return f.Name(), nil
}

// sortCoreGroup sorts the core API group.
func sortCoreGroup(group *metav1.APIResourceList) {
	sort.SliceStable(group.APIResources, func(i, j int) bool {
		return coreGroupResourcePriority(group.APIResources[i].Name) < coreGroupResourcePriority(group.APIResources[j].Name)
	})
}

// These constants represent the relative priorities for resources in the core API group. We want to
// ensure that we process pods, then pvcs, then pvs, then anything else. This ensures that when a
// pod is backed up, we can perform a pre hook, then process pvcs and pvs (including taking a
// snapshot), then perform a post hook on the pod.
const (
	pod = iota
	pvc
	pv
	other
)

// coreGroupResourcePriority returns the relative priority of the resource, in the following order:
// pods, pvcs, pvs, everything else.
func coreGroupResourcePriority(resource string) int {
	switch strings.ToLower(resource) {
	case "pods":
		return pod
	case "persistentvolumeclaims":
		return pvc
	case "persistentvolumes":
		return pv
	}

	return other
}

// getNamespacesToList examines ie and resolves the includes and excludes to a full list of
// namespaces to list. If ie is nil or it includes *, the result is just "" (list across all
// namespaces). Otherwise, the result is a list of every included namespace minus all excluded ones.
func getNamespacesToList(ie *collections.IncludesExcludes) []string {
	if ie == nil {
		return []string{""}
	}

	if ie.ShouldInclude("*") {
		// "" means all namespaces
		return []string{""}
	}

	var list []string
	for _, i := range ie.GetIncludes() {
		if ie.ShouldInclude(i) {
			list = append(list, i)
		}
	}

	return list
}

type cohabitatingResource struct {
	resource       string
	groupResource1 schema.GroupResource
	groupResource2 schema.GroupResource
	seen           bool
}

func newCohabitatingResource(resource, group1, group2 string) *cohabitatingResource {
	return &cohabitatingResource{
		resource:       resource,
		groupResource1: schema.GroupResource{Group: group1, Resource: resource},
		groupResource2: schema.GroupResource{Group: group2, Resource: resource},
		seen:           false,
	}
}
