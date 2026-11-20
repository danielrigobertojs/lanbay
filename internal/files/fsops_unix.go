// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package files

import (
	"io"
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

func copyFileAt(source, parent *pathguard.Resolved, name string, mode os.FileMode) error {
	out, err := os.OpenFile(filepath.Join(parent.Path(), name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, source.File())
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
