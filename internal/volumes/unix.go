// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package volumes

import (
	"context"
	"fmt"
	"syscall"
)

type localProvider struct{}

func NewProvider() Provider { return localProvider{} }
func (localProvider) List(context.Context) ([]Volume, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return nil, fmt.Errorf("stat root filesystem: %w", err)
	}
	size := uint64(stat.Bsize)
	return []Volume{{ID: "/", Label: "Sistema", Type: "fijo", Filesystem: "local", Total: stat.Blocks * size, Free: stat.Bavail * size, Recycle: false}}, nil
}
