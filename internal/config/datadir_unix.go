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
