// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import (
	"os"
	"path/filepath"
	"syscall"
)

func finalPath(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	if _, err := os.Lstat(path); err != nil {
		return "", err
	}
	return path, nil
}

func openRetained(path string) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Best-effort exclusive lock to prevent concurrent writes.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	return f, nil
}
