// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import "context"

func SnapshotUninstallKeys(ctx context.Context) (RegistrySnapshot, error) {
	return RegistrySnapshot{}, nil
}

func SnapshotRegistry(ctx context.Context) (RegistrySnapshot, error) {
	return SnapshotUninstallKeys(ctx)
}
func TakeUninstallSnapshot(ctx context.Context) (RegistrySnapshot, error) {
	return SnapshotUninstallKeys(ctx)
}
func UninstallSnapshot(ctx context.Context) (RegistrySnapshot, error) {
	return SnapshotUninstallKeys(ctx)
}
