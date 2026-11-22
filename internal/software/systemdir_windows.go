// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func systemDirectory() string {
	d, err := windows.GetSystemDirectory()
	if err != nil || d == "" {
		if v := os.Getenv("SystemRoot"); v != "" {
			return filepath.Join(v, "System32")
		}
		return `C:\Windows\System32`
	}
	return d
}
