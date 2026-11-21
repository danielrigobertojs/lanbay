// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type jobObject struct{ handle windows.Handle }

func newJobObject() (*jobObject, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	return &jobObject{handle: h}, nil
}

func (j *jobObject) assign(pid uint32) error {
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(p)
	return windows.AssignProcessToJobObject(j.handle, p)
}

func (j *jobObject) terminate() error {
	return windows.TerminateJobObject(j.handle, 1)
}

func (j *jobObject) close() { windows.CloseHandle(j.handle) }
