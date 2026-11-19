// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

func DefaultDataDir() string {
	if value := os.Getenv("LANBAY_DATA_DIR"); value != "" {
		return value
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ".lanbay"
	}
	return filepath.Join(dir, "Lanbay")
}

func ensureDataDir(path string) error { return os.MkdirAll(path, 0700) }

// ProtectFile applies the development-platform owner-only mode.
func ProtectFile(path string) error { return os.Chmod(path, 0600) }
