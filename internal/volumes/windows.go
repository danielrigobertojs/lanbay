// SPDX-License-Identifier: Apache-2.0
//go:build windows

package volumes

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
	"unsafe"
)

const (
	driveRemovable = 2
	driveFixed     = 3
)

var (
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetLogicalDrives     = kernel32.NewProc("GetLogicalDrives")
	procGetDriveType         = kernel32.NewProc("GetDriveTypeW")
	procQueryDosDevice       = kernel32.NewProc("QueryDosDeviceW")
	procGetDiskFreeSpaceEx   = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetVolumeInformation = kernel32.NewProc("GetVolumeInformationW")
)

type localProvider struct{}

func NewProvider() Provider { return localProvider{} }
func (localProvider) List(context.Context) ([]Volume, error) {
	mask, _, e := procGetLogicalDrives.Call()
	if mask == 0 {
		return nil, fmt.Errorf("GetLogicalDrives: %w", e)
	}
	var volumes []Volume
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+i)
		ptr, _ := windows.UTF16PtrFromString(root)
		typ, _, _ := procGetDriveType.Call(uintptr(unsafe.Pointer(ptr)))
		if typ != driveFixed && typ != driveRemovable {
			continue
		}
		targets, err := dosTargets(fmt.Sprintf("%c:", 'A'+i))
		if err != nil || !acceptedTarget(targets) {
			continue
		}
		var free, total uint64
		r, _, err := procGetDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(ptr)), 0, uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
		if r == 0 {
			continue
		}
		label, fs := volumeInfo(ptr)
		volumes = append(volumes, Volume{ID: root, Label: label, Type: map[uintptr]string{driveFixed: "fijo", driveRemovable: "removible"}[typ], Filesystem: fs, Total: total, Free: free, Recycle: typ == driveFixed && strings.EqualFold(fs, "NTFS")})
	}
	return volumes, nil
}
func dosTargets(name string) ([]string, error) {
	p, _ := windows.UTF16PtrFromString(name)
	b := make([]uint16, 32768)
	r, _, e := procQueryDosDevice.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	if r == 0 {
		return nil, e
	}
	var out []string
	for _, s := range strings.Split(string(windows.UTF16ToString(b[:r])), "\x00") {
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}
func volumeInfo(root *uint16) (string, string) {
	l, f := make([]uint16, 261), make([]uint16, 261)
	r, _, _ := procGetVolumeInformation.Call(uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&l[0])), 261, 0, 0, 0, uintptr(unsafe.Pointer(&f[0])), 261)
	if r == 0 {
		return "", ""
	}
	return windows.UTF16ToString(l), windows.UTF16ToString(f)
}
