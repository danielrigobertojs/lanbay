// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func ReadMSIProductCode(path string) (string, error) {
	msi := syscall.NewLazyDLL("msi.dll")
	proc := msi.NewProc("MsiGetProductCodeW")
	pPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, 64)
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(pPath)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return "", fmt.Errorf("MsiGetProductCodeW returned %d", r)
	}
	return windows.UTF16ToString(buf), nil
}
