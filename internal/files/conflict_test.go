// SPDX-License-Identifier: Apache-2.0
package files

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConflict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, apply, err := ResolveConflict(p, ConflictSkip); err != nil || apply {
		t.Fatalf("skip: %v %v", apply, err)
	}
	got, apply, err := ResolveConflict(p, ConflictRename)
	if err != nil || !apply || filepath.Base(got) != "file (1).txt" {
		t.Fatalf("rename: %q %v %v", got, apply, err)
	}
}
