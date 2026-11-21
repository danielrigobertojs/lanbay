// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package uploads

import "syscall"

func ensureSpace(path string, need int64) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return err
	}
	if int64(stat.Bavail)*int64(stat.Bsize) < need {
		return ErrSpace
	}
	return nil
}
