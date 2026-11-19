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

func TestServiceGuardsAndPaginates(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "Windows")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b.txt", "a.txt", ".hidden"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := Service{Guard: pathguard.Guard{Roots: []string{root}, Protected: []string{protected}}}
	page, err := s.List(context.Background(), root, ListOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Entries) != 1 || !page.Entries[0].Directory {
		t.Fatalf("page: %#v", page)
	}
	if _, err := s.Mkdir(context.Background(), protected, "blocked"); !errors.Is(err, pathguard.ErrProtected) {
		t.Fatalf("mkdir protected: %v", err)
	}
	if _, err := s.Mkdir(context.Background(), root, "created"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rename(context.Background(), filepath.Join(root, "a.txt"), root, "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CopyFile(context.Background(), filepath.Join(root, "renamed.txt"), root, "copy.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "copy.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteFailsClosedWithoutRecycle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "item.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	s := Service{Guard: pathguard.Guard{Roots: []string{root}}}
	if err := s.Delete(context.Background(), path, DeleteOptions{}); !errors.Is(err, ErrRecycleUnavailable) {
		t.Fatalf("delete=%v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file removed: %v", err)
	}
	if err := s.Delete(context.Background(), path, DeleteOptions{Permanent: true}); err != nil {
		t.Fatal(err)
	}
}
