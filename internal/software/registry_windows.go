// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"context"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// uninstallRoots are the three registry locations scanned for uninstall entries.
var uninstallRoots = []struct {
	root registry.Key
	path string
	name string
}{
	{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM"},
	{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`, "HKLM32"},
	{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, "HKCU"},
}

func SnapshotUninstallKeys(ctx context.Context) (RegistrySnapshot, error) {
	snap := RegistrySnapshot{}
	for _, r := range uninstallRoots {
		k, err := registry.OpenKey(r.root, r.path, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		names, err := k.ReadSubKeyNames(0)
		_ = k.Close()
		if err != nil {
			continue
		}
		for _, name := range names {
			full := r.name + `\` + name
			kk, err := registry.OpenKey(r.root, r.path+`\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			e := UninstallEntry{Key: full, Root: r.name}
			if v, _, err := kk.GetStringValue("DisplayName"); err == nil {
				e.DisplayName = v
			}
			if v, _, err := kk.GetStringValue("DisplayVersion"); err == nil {
				e.DisplayVersion = v
			}
			if v, _, err := kk.GetStringValue("Publisher"); err == nil {
				e.Publisher = v
			}
			if v, _, err := kk.GetStringValue("InstallLocation"); err == nil {
				e.InstallLocation = v
			}
			if v, _, err := kk.GetStringValue("QuietUninstallString"); err == nil {
				e.QuietUninstallString = v
			}
			if v, _, err := kk.GetStringValue("UninstallString"); err == nil {
				e.UninstallString = v
			}
			_ = kk.Close()
			if strings.HasPrefix(name, "{") && strings.HasSuffix(name, "}") {
				e.ProductCode = name
			}
			snap[full] = e
		}
	}
	return snap, nil
}

// Aliases.
func SnapshotRegistry(ctx context.Context) (RegistrySnapshot, error) {
	return SnapshotUninstallKeys(ctx)
}
func TakeUninstallSnapshot(ctx context.Context) (RegistrySnapshot, error) {
	return SnapshotUninstallKeys(ctx)
}
func UninstallSnapshot(ctx context.Context) (RegistrySnapshot, error) {
	return SnapshotUninstallKeys(ctx)
}
