// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import "path/filepath"

func finalPath(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	return path, nil
}
