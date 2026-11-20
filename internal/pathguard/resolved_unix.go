// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package pathguard

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func resolveOpen(path string, _ bool, _ bool) (*Resolved, error) {
	p, err := resolve(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	return &Resolved{path: p, file: f}, nil
}

func openChild(parent *Resolved, name string, _ bool, container bool) (*Resolved, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	if container {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(int(parent.File().Fd()), name, flags, 0)
	if err != nil {
		return nil, err
	}
	return &Resolved{path: filepath.Join(parent.Path(), name), file: os.NewFile(uintptr(fd), name)}, nil
}
