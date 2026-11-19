// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
	"golang.org/x/sys/windows"
)

func TestIsSystemReadsWindowsAttribute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "system.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetFileAttributes(pointer, fileAttributeSystem); err != nil {
		t.Fatal(err)
	}
	system, err := isSystem(path)
	if err != nil || !system {
		t.Fatalf("isSystem = %t, %v", system, err)
	}
	m := Manager{Guard: pathguard.Guard{Roots: []string{filepath.Dir(path)}}}
	hidden, err := m.List(context.Background(), filepath.Dir(path), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Total != 0 {
		t.Fatalf("entries without system toggle = %#v", hidden.Entries)
	}
	visible, err := m.List(context.Background(), filepath.Dir(path), ListOptions{System: true})
	if err != nil || visible.Total != 1 || !visible.Entries[0].System {
		t.Fatalf("entries with system toggle = %#v, %v", visible.Entries, err)
	}
}
