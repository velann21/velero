// Code generated by mockery v1.0.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"
import restic "github.com/velann21/velero/pkg/restic"

// Restorer is an autogenerated mock type for the Restorer type
type Restorer struct {
	mock.Mock
}

// RestorePodVolumes provides a mock function with given fields: _a0
func (_m *Restorer) RestorePodVolumes(_a0 restic.RestoreData) []error {
	ret := _m.Called(_a0)

	var r0 []error
	if rf, ok := ret.Get(0).(func(restic.RestoreData) []error); ok {
		r0 = rf(_a0)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]error)
		}
	}

	return r0
}
