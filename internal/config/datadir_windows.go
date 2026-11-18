// SPDX-License-Identifier: Apache-2.0
//go:build windows

package config

import (
	"os"
	"path/filepath"
)

func DefaultDataDir() string {
	if value := os.Getenv("LANBAY_DATA_DIR"); value != "" {
		return value
	}
	dir := os.Getenv("ProgramData")
	if dir == "" {
		dir = `C:\\ProgramData`
	}
	return filepath.Join(dir, "Lanbay")
}
