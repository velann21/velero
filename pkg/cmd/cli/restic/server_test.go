/*
Copyright 2019 the Velero contributors.

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
package restic

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/velann21/velero/pkg/builder"
	testutil "github.com/velann21/velero/pkg/test"
)

func Test_validatePodVolumesHostPath(t *testing.T) {
	tests := []struct {
		name    string
		pods    []*corev1.Pod
		dirs    []string
		wantErr bool
	}{
		{
			name: "no error when pod volumes are present",
			pods: []*corev1.Pod{
				builder.ForPod("foo", "bar").ObjectMeta(builder.WithUID("foo")).Result(),
				builder.ForPod("zoo", "raz").ObjectMeta(builder.WithUID("zoo")).Result(),
			},
			dirs:    []string{"foo", "zoo"},
			wantErr: false,
		},
		{
			name: "no error when pod volumes are present and there are mirror pods",
			pods: []*corev1.Pod{
				builder.ForPod("foo", "bar").ObjectMeta(builder.WithUID("foo")).Result(),
				builder.ForPod("zoo", "raz").ObjectMeta(builder.WithUID("zoo"), builder.WithAnnotations(v1.MirrorPodAnnotationKey, "baz")).Result(),
			},
			dirs:    []string{"foo", "baz"},
			wantErr: false,
		},
		{
			name: "error when all pod volumes missing",
			pods: []*corev1.Pod{
				builder.ForPod("foo", "bar").ObjectMeta(builder.WithUID("foo")).Result(),
				builder.ForPod("zoo", "raz").ObjectMeta(builder.WithUID("zoo")).Result(),
			},
			dirs:    []string{"unexpected-dir"},
			wantErr: true,
		},
		{
			name: "error when some pod volumes missing",
			pods: []*corev1.Pod{
				builder.ForPod("foo", "bar").ObjectMeta(builder.WithUID("foo")).Result(),
				builder.ForPod("zoo", "raz").ObjectMeta(builder.WithUID("zoo")).Result(),
			},
			dirs:    []string{"foo"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := testutil.NewFakeFileSystem()

			for _, dir := range tt.dirs {
				err := fs.MkdirAll(filepath.Join("/host_pods/", dir), os.ModePerm)
				if err != nil {
					t.Error(err)
				}
			}

			kubeClient := fake.NewSimpleClientset()
			for _, pod := range tt.pods {
				_, err := kubeClient.CoreV1().Pods(pod.GetNamespace()).Create(pod)
				if err != nil {
					t.Error(err)
				}
			}

			s := &resticServer{
				kubeClient: kubeClient,
				logger:     testutil.NewLogger(),
				fileSystem: fs,
			}

			err := s.validatePodVolumesHostPath()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
