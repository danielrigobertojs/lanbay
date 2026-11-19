// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestProtectedPathsIncludesDataAndConfiguredDirectories(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	configured := filepath.Join(root, "configured")
	for _, path := range []string{data, configured} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := ProtectedPaths(data, []string{configured})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{data, configured} {
		if !slices.Contains(paths, want) {
			t.Fatalf("protected paths %q do not contain %q", paths, want)
		}
	}
}
