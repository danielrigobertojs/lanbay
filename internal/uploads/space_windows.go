// SPDX-License-Identifier: Apache-2.0
//go:build windows

package uploads

import (
	"golang.org/x/sys/windows"
	"path/filepath"
)

func ensureSpace(path string, need int64) error {
	var available, total, free uint64
	root := filepath.VolumeName(path) + `\`
	if err := windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(root), &available, &total, &free); err != nil {
		return err
	}
	if available < uint64(need) {
		return ErrSpace
	}
	return nil
}
