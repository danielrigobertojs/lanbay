// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"os"

	"golang.org/x/sys/windows"
)

func finalPath(path string) (string, error) {
	flags := uint32(0)
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		flags = windows.FILE_FLAG_BACKUP_SEMANTICS
	}
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1<<16)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// openRetained opens an installer with FILE_SHARE_READ only, preventing writes
// or deletion while the handle is held (INS-2).
func openRetained(path string) (*os.File, error) {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
