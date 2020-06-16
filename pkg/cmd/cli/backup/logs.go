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
	"os"
	"time"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/velann21/velero/pkg/apis/velero/v1"
	"github.com/velann21/velero/pkg/client"
	"github.com/velann21/velero/pkg/cmd"
	"github.com/velann21/velero/pkg/cmd/util/downloadrequest"
)

func NewLogsCommand(f client.Factory) *cobra.Command {
	config, err := client.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error reading config file: %v\n", err)
	}

	timeout := time.Minute
	insecureSkipTLSVerify := false
	caCertFile := config.CACertFile()

	c := &cobra.Command{
		Use:   "logs BACKUP",
		Short: "Get backup logs",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			backupName := args[0]

			veleroClient, err := f.Client()
			cmd.CheckError(err)

			backup, err := veleroClient.VeleroV1().Backups(f.Namespace()).Get(backupName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				cmd.Exit("Backup %q does not exist.", backupName)
			} else if err != nil {
				cmd.Exit("Error checking for backup %q: %v", backupName, err)
			}

			switch backup.Status.Phase {
			case v1.BackupPhaseCompleted, v1.BackupPhasePartiallyFailed, v1.BackupPhaseFailed:
				// terminal phases, do nothing.
			default:
				cmd.Exit("Logs for backup %q are not available until it's finished processing. Please wait "+
					"until the backup has a phase of Completed or Failed and try again.", backupName)
			}

			err = downloadrequest.Stream(veleroClient.VeleroV1(), f.Namespace(), backupName, v1.DownloadTargetKindBackupLog, os.Stdout, timeout, insecureSkipTLSVerify, caCertFile)
			cmd.CheckError(err)
		},
	}

	c.Flags().DurationVar(&timeout, "timeout", timeout, "how long to wait to receive logs")
	c.Flags().BoolVar(&insecureSkipTLSVerify, "insecure-skip-tls-verify", insecureSkipTLSVerify, "If true, the object store's TLS certificate will not be checked for validity. This is insecure and susceptible to man-in-the-middle attacks. Not recommended for production.")
	c.Flags().StringVar(&caCertFile, "cacert", caCertFile, "path to a certificate bundle to use when verifying TLS connections")
	return c
}
