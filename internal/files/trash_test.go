// SPDX-License-Identifier: Apache-2.0
package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

type recordingTrash struct{ path string }

func (t *recordingTrash) Delete(_ context.Context, path string) error { t.path = path; return nil }

func TestDeleteRequiresPermanentConfirmationWithoutRecycle(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "delete-me.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}}}
	if err := m.Delete(context.Background(), file, DeleteOptions{}); !errors.Is(err, ErrRecycleUnavailable) {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("file must remain: %v", err)
	}
	if err := m.Delete(context.Background(), file, DeleteOptions{Permanent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("file must be deleted: %v", err)
	}
}

func TestDeleteUsesTrashOnlyWhenCapabilityAllowsIt(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "delete-me.txt")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	trash := &recordingTrash{}
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}}, Trash: trash}
	if err := m.Delete(context.Background(), file, DeleteOptions{Recycle: true}); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatal(err)
	}
	if trash.path != want {
		t.Fatalf("trash got %q, want %q", trash.path, want)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("trash fake must retain file: %v", err)
	}
}
