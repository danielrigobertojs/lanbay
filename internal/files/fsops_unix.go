// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package files

import (
	"os"
	"path/filepath"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

func mkdirAt(parent *pathguard.Resolved, name string, mode os.FileMode) error {
	return os.Mkdir(filepath.Join(parent.Path(), name), mode)
}

func renameAt(source, parent *pathguard.Resolved, name string) error {
	return os.Rename(source.Path(), filepath.Join(parent.Path(), name))
}

func deleteAt(resolved *pathguard.Resolved) error {
	info, err := resolved.File().Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ErrRecursiveDelete
	}
	return os.Remove(resolved.Path())
}
