// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

func TestDeleteReadonlyFileByHandle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "readonly.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}}}
	if err := m.Delete(context.Background(), path, DeleteOptions{Permanent: true}); err != nil {
		t.Fatalf("delete readonly file: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("readonly file remains: %v", err)
	}
}

func TestRenameReadonlyFileByHandle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "readonly.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}}}
	if _, err := m.Rename(context.Background(), path, root, "renamed.txt"); err != nil {
		t.Fatalf("rename readonly file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatalf("renamed readonly file: %v", err)
	}
}
