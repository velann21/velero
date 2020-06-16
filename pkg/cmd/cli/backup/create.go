/*
Copyright 2017 the Velero contributors.

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
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	velerov1api "github.com/velann21/velero/pkg/apis/velero/v1"
	"github.com/velann21/velero/pkg/builder"
	"github.com/velann21/velero/pkg/client"
	"github.com/velann21/velero/pkg/cmd"
	"github.com/velann21/velero/pkg/cmd/util/flag"
	"github.com/velann21/velero/pkg/cmd/util/output"
	veleroclient "github.com/velann21/velero/pkg/generated/clientset/versioned"
	v1 "github.com/velann21/velero/pkg/generated/informers/externalversions/velero/v1"
)

const DefaultBackupTTL time.Duration = 30 * 24 * time.Hour

func NewCreateCommand(f client.Factory, use string) *cobra.Command {
	o := NewCreateOptions()

	c := &cobra.Command{
		Use:   use + " NAME",
		Short: "Create a backup",
		Args:  cobra.MaximumNArgs(1),
		Run: func(c *cobra.Command, args []string) {
			cmd.CheckError(o.Complete(args, f))
			cmd.CheckError(o.Validate(c, args, f))
			cmd.CheckError(o.Run(c, f))
		},
		Example: `	# create a backup containing all resources
	velero backup create backup1

	# create a backup including only the nginx namespace
	velero backup create nginx-backup --include-namespaces nginx

	# create a backup excluding the velero and default namespaces
	velero backup create backup2 --exclude-namespaces velero,default

	# create a backup based on a schedule named daily-backup
	velero backup create --from-schedule daily-backup

	# view the YAML for a backup that doesn't snapshot volumes, without sending it to the server
	velero backup create backup3 --snapshot-volumes=false -o yaml

	# wait for a backup to complete before returning from the command
	velero backup create backup4 --wait`,
	}

	o.BindFlags(c.Flags())
	o.BindWait(c.Flags())
	o.BindFromSchedule(c.Flags())
	output.BindFlags(c.Flags())
	output.ClearOutputFlagDefault(c)

	return c
}

type CreateOptions struct {
	Name                    string
	TTL                     time.Duration
	SnapshotVolumes         flag.OptionalBool
	IncludeNamespaces       flag.StringArray
	ExcludeNamespaces       flag.StringArray
	IncludeResources        flag.StringArray
	ExcludeResources        flag.StringArray
	Labels                  flag.Map
	Selector                flag.LabelSelector
	IncludeClusterResources flag.OptionalBool
	Wait                    bool
	StorageLocation         string
	SnapshotLocations       []string
	FromSchedule            string

	client veleroclient.Interface
}

func NewCreateOptions() *CreateOptions {
	return &CreateOptions{
		TTL:                     DefaultBackupTTL,
		IncludeNamespaces:       flag.NewStringArray("*"),
		Labels:                  flag.NewMap(),
		SnapshotVolumes:         flag.NewOptionalBool(nil),
		IncludeClusterResources: flag.NewOptionalBool(nil),
	}
}

func (o *CreateOptions) BindFlags(flags *pflag.FlagSet) {
	flags.DurationVar(&o.TTL, "ttl", o.TTL, "how long before the backup can be garbage collected")
	flags.Var(&o.IncludeNamespaces, "include-namespaces", "namespaces to include in the backup (use '*' for all namespaces)")
	flags.Var(&o.ExcludeNamespaces, "exclude-namespaces", "namespaces to exclude from the backup")
	flags.Var(&o.IncludeResources, "include-resources", "resources to include in the backup, formatted as resource.group, such as storageclasses.storage.k8s.io (use '*' for all resources)")
	flags.Var(&o.ExcludeResources, "exclude-resources", "resources to exclude from the backup, formatted as resource.group, such as storageclasses.storage.k8s.io")
	flags.Var(&o.Labels, "labels", "labels to apply to the backup")
	flags.StringVar(&o.StorageLocation, "storage-location", "", "location in which to store the backup")
	flags.StringSliceVar(&o.SnapshotLocations, "volume-snapshot-locations", o.SnapshotLocations, "list of locations (at most one per provider) where volume snapshots should be stored")
	flags.VarP(&o.Selector, "selector", "l", "only back up resources matching this label selector")
	f := flags.VarPF(&o.SnapshotVolumes, "snapshot-volumes", "", "take snapshots of PersistentVolumes as part of the backup")
	// this allows the user to just specify "--snapshot-volumes" as shorthand for "--snapshot-volumes=true"
	// like a normal bool flag
	f.NoOptDefVal = "true"

	f = flags.VarPF(&o.IncludeClusterResources, "include-cluster-resources", "", "include cluster-scoped resources in the backup")
	f.NoOptDefVal = "true"
}

// BindWait binds the wait flag separately so it is not called by other create
// commands that reuse CreateOptions's BindFlags method.
func (o *CreateOptions) BindWait(flags *pflag.FlagSet) {
	flags.BoolVarP(&o.Wait, "wait", "w", o.Wait, "wait for the operation to complete")
}

// BindFromSchedule binds the from-schedule flag separately so it is not called
// by other create commands that reuse CreateOptions's BindFlags method.
func (o *CreateOptions) BindFromSchedule(flags *pflag.FlagSet) {
	flags.StringVar(&o.FromSchedule, "from-schedule", "", "create a backup from the template of an existing schedule. Cannot be used with any other filters. Backup name is optional if used.")
}

func (o *CreateOptions) Validate(c *cobra.Command, args []string, f client.Factory) error {
	if err := output.ValidateFlags(c); err != nil {
		return err
	}

	// Ensure that unless FromSchedule is set, args contains a backup name
	if o.FromSchedule == "" && len(args) != 1 {
		return fmt.Errorf("a backup name is required, unless you are creating based on a schedule")
	}

	if o.StorageLocation != "" {
		if _, err := o.client.VeleroV1().BackupStorageLocations(f.Namespace()).Get(o.StorageLocation, metav1.GetOptions{}); err != nil {
			return err
		}
	}

	for _, loc := range o.SnapshotLocations {
		if _, err := o.client.VeleroV1().VolumeSnapshotLocations(f.Namespace()).Get(loc, metav1.GetOptions{}); err != nil {
			return err
		}
	}

	return nil
}

func (o *CreateOptions) Complete(args []string, f client.Factory) error {
	// If an explict name is specified, use that name
	if len(args) > 0 {
		o.Name = args[0]
	}
	client, err := f.Client()
	if err != nil {
		return err
	}
	o.client = client
	return nil
}

func (o *CreateOptions) Run(c *cobra.Command, f client.Factory) error {
	backup, err := o.BuildBackup(f.Namespace())
	if err != nil {
		return err
	}

	if printed, err := output.PrintWithFormat(c, backup); printed || err != nil {
		return err
	}

	if o.FromSchedule != "" {
		fmt.Println("Creating backup from schedule, all other filters are ignored.")
	}

	var backupInformer cache.SharedIndexInformer
	var updates chan *velerov1api.Backup
	if o.Wait {
		stop := make(chan struct{})
		defer close(stop)

		updates = make(chan *velerov1api.Backup)

		backupInformer = v1.NewBackupInformer(o.client, f.Namespace(), 0, nil)

		backupInformer.AddEventHandler(
			cache.FilteringResourceEventHandler{
				FilterFunc: func(obj interface{}) bool {
					backup, ok := obj.(*velerov1api.Backup)
					if !ok {
						return false
					}
					return backup.Name == o.Name
				},
				Handler: cache.ResourceEventHandlerFuncs{
					UpdateFunc: func(_, obj interface{}) {
						backup, ok := obj.(*velerov1api.Backup)
						if !ok {
							return
						}
						updates <- backup
					},
					DeleteFunc: func(obj interface{}) {
						backup, ok := obj.(*velerov1api.Backup)
						if !ok {
							return
						}
						updates <- backup
					},
				},
			},
		)
		go backupInformer.Run(stop)
	}

	_, err = o.client.VeleroV1().Backups(backup.Namespace).Create(backup)
	if err != nil {
		return err
	}

	fmt.Printf("Backup request %q submitted successfully.\n", backup.Name)
	if o.Wait {
		fmt.Println("Waiting for backup to complete. You may safely press ctrl-c to stop waiting - your backup will continue in the background.")
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fmt.Print(".")
			case backup, ok := <-updates:
				if !ok {
					fmt.Println("\nError waiting: unable to watch backups.")
					return nil
				}

				if backup.Status.Phase != velerov1api.BackupPhaseNew && backup.Status.Phase != velerov1api.BackupPhaseInProgress {
					fmt.Printf("\nBackup completed with status: %s. You may check for more information using the commands `velero backup describe %s` and `velero backup logs %s`.\n", backup.Status.Phase, backup.Name, backup.Name)
					return nil
				}
			}
		}
	}

	// Not waiting

	fmt.Printf("Run `velero backup describe %s` or `velero backup logs %s` for more details.\n", backup.Name, backup.Name)

	return nil
}

func (o *CreateOptions) BuildBackup(namespace string) (*velerov1api.Backup, error) {
	var backupBuilder *builder.BackupBuilder

	if o.FromSchedule != "" {
		schedule, err := o.client.VeleroV1().Schedules(namespace).Get(o.FromSchedule, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		if o.Name == "" {
			o.Name = schedule.TimestampedName(time.Now())
		}
		backupBuilder = builder.ForBackup(namespace, o.Name).
			FromSchedule(schedule)
	} else {
		backupBuilder = builder.ForBackup(namespace, o.Name).
			IncludedNamespaces(o.IncludeNamespaces...).
			ExcludedNamespaces(o.ExcludeNamespaces...).
			IncludedResources(o.IncludeResources...).
			ExcludedResources(o.ExcludeResources...).
			LabelSelector(o.Selector.LabelSelector).
			TTL(o.TTL).
			StorageLocation(o.StorageLocation).
			VolumeSnapshotLocations(o.SnapshotLocations...)

		if o.SnapshotVolumes.Value != nil {
			backupBuilder.SnapshotVolumes(*o.SnapshotVolumes.Value)
		}
		if o.IncludeClusterResources.Value != nil {
			backupBuilder.IncludeClusterResources(*o.IncludeClusterResources.Value)
		}
	}

	backup := backupBuilder.ObjectMeta(builder.WithLabelsMap(o.Labels.Data())).Result()
	return backup, nil
}



func CreateBackup(f client.Factory, o *CreateOptions, result chan *velerov1api.Backup)error{
	backup, err := o.BuildBackup(f.Namespace())
	if err != nil {
		return err
	}

	var backupInformer cache.SharedIndexInformer
	var updates chan *velerov1api.Backup
	if o.Wait {
		stop := make(chan struct{})
		defer close(stop)

		updates = make(chan *velerov1api.Backup)

		backupInformer = v1.NewBackupInformer(o.client, f.Namespace(), 0, nil)

		backupInformer.AddEventHandler(
			cache.FilteringResourceEventHandler{
				FilterFunc: func(obj interface{}) bool {
					backup, ok := obj.(*velerov1api.Backup)
					if !ok {
						return false
					}
					return backup.Name == o.Name
				},
				Handler: cache.ResourceEventHandlerFuncs{
					UpdateFunc: func(_, obj interface{}) {
						backup, ok := obj.(*velerov1api.Backup)
						if !ok {
							return
						}
						updates <- backup
					},
					DeleteFunc: func(obj interface{}) {
						backup, ok := obj.(*velerov1api.Backup)
						if !ok {
							return
						}
						updates <- backup
					},
				},
			},
		)
		go backupInformer.Run(stop)
	}

	_, err = o.client.VeleroV1().Backups(backup.Namespace).Create(backup)
	if err != nil {
		return err
	}

	if o.Wait {
		fmt.Println("Waiting for backup to complete. You may safely press ctrl-c to stop waiting - your backup will continue in the background.")
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				fmt.Print(".")
			case backup, ok := <-updates:
				if !ok {
					fmt.Println("\nError waiting: unable to watch backups.")
					return nil
				}

				if backup.Status.Phase != velerov1api.BackupPhaseNew && backup.Status.Phase != velerov1api.BackupPhaseInProgress {
					fmt.Printf("\nBackup completed with status: %s. You may check for more information using the commands `velero backup describe %s` and `velero backup logs %s`.\n", backup.Status.Phase, backup.Name, backup.Name)
					result <- backup
					return nil
				}
			}
		}
	}

	fmt.Printf("Run `velero backup describe %s` or `velero backup logs %s` for more details.\n", backup.Name, backup.Name)
	return nil
}