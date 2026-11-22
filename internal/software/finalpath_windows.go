// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func finalPath(path string) (string, error) {
	h, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 1<<16)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil || n == 0 {
		return "", fmt.Errorf("GetFinalPathNameByHandle: %w", err)
	}
	return windows.UTF16ToString(buf[:n]), nil
}
