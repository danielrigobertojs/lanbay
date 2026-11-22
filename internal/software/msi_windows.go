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
	open := msi.NewProc("MsiOpenPackageExW")
	getProp := msi.NewProc("MsiGetProductPropertyW")
	closeP := msi.NewProc("MsiCloseHandle")
	pPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	var h uintptr
	r, _, _ := open.Call(uintptr(unsafe.Pointer(pPath)), 0, uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return "", fmt.Errorf("MsiOpenPackageExW returned %d", r)
	}
	defer closeP.Call(h)
	buf := make([]uint16, 256)
	size := uint32(len(buf))
	r, _, _ = getProp.Call(h, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("ProductCode"))), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return "", fmt.Errorf("MsiGetProductPropertyW returned %d", r)
	}
	return windows.UTF16ToString(buf[:size]), nil
}
