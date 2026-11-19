// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

// ProtectedPaths returns the operating-system directories which Lanbay must
// never mutate. Development systems keep an equivalent conservative set.
func ProtectedPaths(dataDir string, configured []string) ([]string, error) {
	system, err := existingPaths([]string{"/System", "/Library", "/Applications", "/usr", "/bin", "/sbin", "/etc"}, true)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	required := append([]string{dataDir, filepath.Dir(executable)}, configured...)
	paths, err := existingPaths(required, false)
	if err != nil {
		return nil, err
	}
	return append(system, paths...), nil
}

func existingPaths(paths []string, skipMissing bool) ([]string, error) {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("make protected path absolute: %w", err)
		}
		if _, err := os.Stat(absolute); err != nil {
			if skipMissing && os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat protected path %q: %w", absolute, err)
		}
		if _, ok := seen[absolute]; ok {
			continue
		}
		seen[absolute] = struct{}{}
		result = append(result, absolute)
	}
	return result, nil
}
