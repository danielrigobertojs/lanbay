// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package files

import (
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

func mkdirAt(parent *pathguard.Resolved, name string, mode os.FileMode) error {
	return unix.Mkdirat(int(parent.File().Fd()), name, uint32(mode))
}

func renameAt(source, parent *pathguard.Resolved, name string) error {
	oldName := filepath.Base(source.Path())
	if source.ParentFile() != nil {
		if err := unix.Renameat(int(source.ParentFile().Fd()), oldName, int(parent.File().Fd()), name); err == nil {
			return nil
		}
	}
	return os.Rename(source.Path(), filepath.Join(parent.Path(), name))
}

func deleteAt(resolved *pathguard.Resolved) error {
	if resolved == nil {
		return os.ErrInvalid
	}
	parent := resolved.ParentFile()
	if parent == nil {
		return os.Remove(resolved.Path())
	}
	name := filepath.Base(resolved.Path())
	flags := 0
	if resolved.File() != nil {
		if info, err := resolved.File().Stat(); err == nil && info.IsDir() {
			flags = unix.AT_REMOVEDIR
		}
	}
	return unix.Unlinkat(int(parent.Fd()), name, flags)
}

func copyFileAt(source, parent *pathguard.Resolved, name string, mode os.FileMode) error {
	fd, err := unix.Openat(int(parent.File().Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, uint32(mode))
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(fd), name)
	_, copyErr := io.Copy(out, source.File())
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
