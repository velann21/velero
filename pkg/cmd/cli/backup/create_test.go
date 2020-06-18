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

package backup

import (
	"github.com/velann21/velero/pkg/client"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	velerov1api "github.com/velann21/velero/pkg/apis/velero/v1"
	"github.com/velann21/velero/pkg/builder"
	"github.com/velann21/velero/pkg/generated/clientset/versioned/fake"
)

const testNamespace = "velero"

func TestCreateOptions_BuildBackup(t *testing.T) {
	o := NewCreateOptions()
	o.Labels.Set("velero.io/test=true")

	backup, err := o.BuildBackup(testNamespace)
	assert.NoError(t, err)

	assert.Equal(t, velerov1api.BackupSpec{
		TTL:                     metav1.Duration{Duration: o.TTL},
		IncludedNamespaces:      []string(o.IncludeNamespaces),
		SnapshotVolumes:         o.SnapshotVolumes.Value,
		IncludeClusterResources: o.IncludeClusterResources.Value,
	}, backup.Spec)

	assert.Equal(t, map[string]string{
		"velero.io/test": "true",
	}, backup.GetLabels())
}

func TestCreateOptions_BuildBackupFromSchedule(t *testing.T) {
	o := NewCreateOptions()
	o.FromSchedule = "test"
	o.client = fake.NewSimpleClientset()

	t.Run("inexistant schedule", func(t *testing.T) {
		_, err := o.BuildBackup(testNamespace)
		assert.Error(t, err)
	})

	expectedBackupSpec := builder.ForBackup("test", testNamespace).IncludedNamespaces("test").Result().Spec
	schedule := builder.ForSchedule(testNamespace, "test").Template(expectedBackupSpec).ObjectMeta(builder.WithLabels("velero.io/test", "true")).Result()
	o.client.VeleroV1().Schedules(testNamespace).Create(schedule)

	t.Run("existing schedule", func(t *testing.T) {
		backup, err := o.BuildBackup(testNamespace)
		assert.NoError(t, err)

		assert.Equal(t, expectedBackupSpec, backup.Spec)
		assert.Equal(t, map[string]string{
			"velero.io/test":              "true",
			velerov1api.ScheduleNameLabel: "test",
		}, backup.GetLabels())
	})

	t.Run("command line labels take precedence over schedule labels", func(t *testing.T) {
		o.Labels.Set("velero.io/test=yes,custom-label=true")
		backup, err := o.BuildBackup(testNamespace)
		assert.NoError(t, err)

		assert.Equal(t, expectedBackupSpec, backup.Spec)
		assert.Equal(t, map[string]string{
			"velero.io/test":              "yes",
			velerov1api.ScheduleNameLabel: "test",
			"custom-label":                "true",
		}, backup.GetLabels())
	})
}


func Test_Backup(t *testing.T){
	o := NewCreateOptions()
	f := client.NewSDKFactory("backup", "/Users/singaravelannandakumar/.kube/config1", "", nil, "velero")
	//f := client.NewFactory("backup", nil)
	fclient, err := f.Client()
	if err != nil{

	}
	o.client = fclient
	o.IncludeNamespaces = []string{"nginx-example"}
    o.Name = "rcvelannewbackup12"
	res := make(chan *velerov1api.Backup)
    err = CreateBackup(f, o,res)
    if err != nil{

	}

}

func Test_DescribeBackup(t *testing.T){
	o := NewCreateOptions()
	f := client.NewFactory("backup", nil)
	fclient, err := f.Client()
	if err != nil{

	}

	o.client = fclient
	args := []string{"velannewbackup"}
	listOptions := metav1.ListOptions{}
	DescribeFunction(f, args, listOptions, false, false, "")
}

func Test_GetBackup(t *testing.T){
	o := NewCreateOptions()
	f := client.NewFactory("backup", nil)
	fclient, err := f.Client()
	if err != nil{

	}
	o.client = fclient
	args := []string{"velannewbackup"}
	listOptions := metav1.ListOptions{}
    GetBackupFunction(f, args, listOptions)
}
