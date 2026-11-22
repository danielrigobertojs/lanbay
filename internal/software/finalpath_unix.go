// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import "os"

func finalPath(path string) (string, error) {
	if _, err := os.Lstat(path); err != nil {
		return "", err
	}
	return path, nil
}
