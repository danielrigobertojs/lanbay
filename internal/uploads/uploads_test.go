// SPDX-License-Identifier: Apache-2.0
package uploads

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
	"github.com/danielrigobertojs/lanbay/internal/pathguard"
	"github.com/danielrigobertojs/lanbay/internal/store"
)

func manager(t *testing.T) (*Manager, pathguard.Guard) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(root, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	jm, err := jobs.New(db)
	if err != nil {
		t.Fatal(err)
	}
	return New(db, jm), pathguard.Guard{Roots: []string{root}}
}
func TestResumeAndAtomicFinalize(t *testing.T) {
	m, g := manager(t)
	ctx := context.Background()
	s, err := m.Create(ctx, g, g.Roots[0], "movie.bin", 2*MinChunk, MinChunk)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Write(ctx, s.ID, 0, MinChunk, bytes.NewReader(bytes.Repeat([]byte{'a'}, MinChunk))); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Write(ctx, s.ID, MinChunk, MinChunk, bytes.NewReader(bytes.Repeat([]byte{'b'}, MinChunk))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(g.Roots[0], "movie.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2*MinChunk || got[0] != 'a' || got[len(got)-1] != 'b' {
		t.Fatal("wrong resumed content")
	}
}
func TestCancelNeverCreatesFinalFile(t *testing.T) {
	m, g := manager(t)
	ctx := context.Background()
	s, err := m.Create(ctx, g, g.Roots[0], "cancel.bin", 2*MinChunk, MinChunk)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Write(ctx, s.ID, 0, MinChunk, bytes.NewReader(bytes.Repeat([]byte{'x'}, MinChunk))); err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(g.Roots[0], "cancel.bin"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final file exists: %v", err)
	}
}
func TestRejectsUnexpectedOffset(t *testing.T) {
	m, g := manager(t)
	s, err := m.Create(context.Background(), g, g.Roots[0], "offset.bin", MinChunk, MinChunk)
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Write(context.Background(), s.ID, 1, MinChunk, bytes.NewReader(bytes.Repeat([]byte{'x'}, MinChunk)))
	if !errors.Is(err, ErrOffset) {
		t.Fatalf("error=%v", err)
	}
}

func TestAcceptsSmallFinalFile(t *testing.T) {
	m, g := manager(t)
	s, err := m.Create(context.Background(), g, g.Roots[0], "small.bin", 3, MinChunk)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write(context.Background(), s.ID, 0, 3, bytes.NewReader([]byte("abc"))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(g.Roots[0], "small.bin"))
	if err != nil || string(got) != "abc" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func BenchmarkChunkWrite(b *testing.B) {
	root := b.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(root, "benchmark.sqlite"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	jm, err := jobs.New(db)
	if err != nil {
		b.Fatal(err)
	}
	m := New(db, jm)
	g := pathguard.Guard{Roots: []string{root}}
	chunk := bytes.Repeat([]byte{'x'}, MinChunk)
	b.SetBytes(MinChunk)
	for i := 0; i < b.N; i++ {
		s, err := m.Create(context.Background(), g, root, "benchmark-"+strconv.Itoa(i)+".bin", MinChunk, MinChunk)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := m.Write(context.Background(), s.ID, 0, MinChunk, bytes.NewReader(chunk)); err != nil {
			b.Fatal(err)
		}
	}
}
