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

func TestMkdirAndCopyByHandle(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	if err := os.WriteFile(src, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}}}

	// Mkdir must succeed on a directory opened as a container handle.
	if _, err := m.Mkdir(context.Background(), root, "sub"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "sub")); err != nil || !info.IsDir() {
		t.Fatalf("created directory missing: %v", err)
	}

	// Copy must create the destination through the validated parent handle.
	if err := m.Copy(context.Background(), src, root, "dst.txt"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "dst.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("copied file: %v %s", err, data)
	}
}

func TestMoveBetweenDirectoriesByHandle(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "a"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "b"), 0700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "a", "file.txt")
	if err := os.WriteFile(src, []byte("move"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}}}
	if err := m.Move(context.Background(), src, filepath.Join(root, "b"), "file.txt"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source remains after move: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "b", "file.txt")); err != nil || string(data) != "move" {
		t.Fatalf("moved file: %v %s", err, data)
	}
}
