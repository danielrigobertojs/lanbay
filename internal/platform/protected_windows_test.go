// SPDX-License-Identifier: Apache-2.0
//go:build windows

package platform

import (
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProtectedPathsIncludesKnownWindowsFolders(t *testing.T) {
	data := t.TempDir()
	paths, err := ProtectedPaths(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, folder := range []*windows.KNOWNFOLDERID{windows.FOLDERID_Windows, windows.FOLDERID_ProgramFiles, windows.FOLDERID_ProgramData, windows.FOLDERID_UserProfiles} {
		want, err := windows.KnownFolderPath(folder, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !containsFold(paths, want) {
			t.Fatalf("protected paths %q do not contain %q", paths, want)
		}
	}
}

func containsFold(paths []string, want string) bool {
	for _, path := range paths {
		if strings.EqualFold(filepath.Clean(path), filepath.Clean(want)) {
			return true
		}
	}
	return false
}
