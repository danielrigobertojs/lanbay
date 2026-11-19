// SPDX-License-Identifier: Apache-2.0
//go:build windows

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// ProtectedPaths obtains protected Windows locations from the shell instead of
// assuming a particular system drive. A missing known folder fails closed.
func ProtectedPaths(dataDir string, configured []string) ([]string, error) {
	folders := []*windows.KNOWNFOLDERID{
		windows.FOLDERID_Windows,
		windows.FOLDERID_ProgramFiles,
		windows.FOLDERID_ProgramFilesX86,
		windows.FOLDERID_ProgramData,
		windows.FOLDERID_UserProfiles,
	}
	paths := make([]string, 0, len(folders)+len(configured)+2)
	for _, folder := range folders {
		path, err := windows.KnownFolderPath(folder, 0)
		if err != nil {
			return nil, fmt.Errorf("resolve known protected folder: %w", err)
		}
		paths = append(paths, path)
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	paths = append(paths, dataDir, filepath.Dir(executable))
	paths = append(paths, configured...)
	return existingPaths(paths, false)
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
