// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import "golang.org/x/sys/windows"

const fileAttributeSystem = 0x4

func isSystem(path string) (bool, error) {
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attributes, err := windows.GetFileAttributes(pointer)
	if err != nil {
		return false, err
	}
	return attributes&fileAttributeSystem != 0, nil
}
