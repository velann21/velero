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
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/velann21/velero/pkg/apis/velero/v1"
	"github.com/velann21/velero/pkg/client"
	"github.com/velann21/velero/pkg/cmd"
	"github.com/velann21/velero/pkg/cmd/util/output"
)

func NewGetCommand(f client.Factory, use string) *cobra.Command {
	var listOptions metav1.ListOptions

	c := &cobra.Command{
		Use:   use,
		Short: "Get backups",
		Run: func(c *cobra.Command, args []string) {
			err := output.ValidateFlags(c)
			cmd.CheckError(err)

			veleroClient, err := f.Client()
			cmd.CheckError(err)

			var backups *api.BackupList
			if len(args) > 0 {
				backups = new(api.BackupList)
				for _, name := range args {
					backup, err := veleroClient.VeleroV1().Backups(f.Namespace()).Get(name, metav1.GetOptions{})
					cmd.CheckError(err)
					backups.Items = append(backups.Items, *backup)
				}
			} else {
				backups, err = veleroClient.VeleroV1().Backups(f.Namespace()).List(listOptions)
				cmd.CheckError(err)
			}

			_, err = output.PrintWithFormat(c, backups)
			cmd.CheckError(err)
		},
	}

	c.Flags().StringVarP(&listOptions.LabelSelector, "selector", "l", listOptions.LabelSelector, "only show items matching this label selector")

	output.BindFlags(c.Flags())

	return c
}

func GetBackupFunction(f client.Factory, args []string, listOptions metav1.ListOptions){
	veleroClient, err := f.Client()
	var backups *api.BackupList
	if len(args) > 0 {
		backups = new(api.BackupList)
		for _, name := range args {
			backup, err := veleroClient.VeleroV1().Backups(f.Namespace()).Get(name, metav1.GetOptions{})
			cmd.CheckError(err)
			backups.Items = append(backups.Items, *backup)
		}
	} else {
		backups, err = veleroClient.VeleroV1().Backups(f.Namespace()).List(listOptions)
		cmd.CheckError(err)
	}

	for _, v := range backups.Items{
		status := v.Status
		fmt.Println(status.Phase)
	}
	cmd.CheckError(err)
}
