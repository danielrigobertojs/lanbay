// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import (
	"os"
	"path/filepath"
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
