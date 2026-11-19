// SPDX-License-Identifier: Apache-2.0
// Package files implements guarded file operations for the HTTP API.
package files

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

type Entry struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size"`
	Modified  time.Time `json:"modified"`
	Hidden    bool      `json:"hidden"`
}
type ListOptions struct {
	Page, PerPage  int
	Sort           string
	Hidden, System bool
}
type Listing struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
	Page    int     `json:"page"`
	PerPage int     `json:"per_page"`
}
type Manager struct{ Guard pathguard.Guard }

func (m Manager) List(ctx context.Context, client string, opt ListOptions) (Listing, error) {
	p, err := m.Guard.Check(client, false)
	if err != nil {
		return Listing{}, err
	}
	dirs, err := os.ReadDir(p)
	if err != nil {
		return Listing{}, fmt.Errorf("read directory: %w", err)
	}
	items := make([]Entry, 0, len(dirs))
	for _, d := range dirs {
		if err := ctx.Err(); err != nil {
			return Listing{}, err
		}
		if !opt.Hidden && strings.HasPrefix(d.Name(), ".") {
			continue
		}
		info, e := d.Info()
		if e != nil {
			continue
		}
		items = append(items, Entry{Name: d.Name(), Path: filepath.Join(p, d.Name()), Directory: d.IsDir(), Size: info.Size(), Modified: info.ModTime(), Hidden: strings.HasPrefix(d.Name(), ".")})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Directory != items[j].Directory {
			return items[i].Directory
		}
		if opt.Sort == "modified" {
			return items[i].Modified.After(items[j].Modified)
		}
		if opt.Sort == "size" {
			return items[i].Size > items[j].Size
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	if opt.PerPage < 1 || opt.PerPage > 1000 {
		opt.PerPage = 100
	}
	if opt.Page < 1 {
		opt.Page = 1
	}
	start := (opt.Page - 1) * opt.PerPage
	if start > len(items) {
		start = len(items)
	}
	end := start + opt.PerPage
	if end > len(items) {
		end = len(items)
	}
	return Listing{Entries: items[start:end], Total: len(items), Page: opt.Page, PerPage: opt.PerPage}, nil
}
func (m Manager) Mkdir(ctx context.Context, parent, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return err
	}
	return os.Mkdir(p, 0750)
}
func (m Manager) Rename(ctx context.Context, source, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := m.Guard.Check(source, true)
	if err != nil {
		return err
	}
	dst, err := m.Guard.CheckNew(filepath.Dir(src), name, true)
	if err != nil {
		return err
	}
	return os.Rename(src, dst)
}
func (m Manager) Move(ctx context.Context, source, parent, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := m.Guard.Check(source, true)
	if err != nil {
		return err
	}
	dst, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return err
	}
	return os.Rename(src, dst)
}
func (m Manager) Copy(ctx context.Context, source, parent, name string) error {
	src, err := m.Guard.Check(source, false)
	if err != nil {
		return err
	}
	dst, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return err
	}
	return copyPath(ctx, src, dst)
}
func (m Manager) Delete(ctx context.Context, source string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := m.Guard.Check(source, true)
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}
func copyPath(ctx context.Context, src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err = os.Mkdir(dst, info.Mode()); err != nil {
			return err
		}
		items, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := copyPath(ctx, filepath.Join(src, item.Name()), filepath.Join(dst, item.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode())
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (m Manager) Zip(ctx context.Context, client string, out io.Writer) error {
	p, err := m.Guard.Check(client, false)
	if err != nil {
		return err
	}
	z := zip.NewWriter(out)
	defer z.Close()
	return filepath.Walk(p, func(name string, info os.FileInfo, e error) error {
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Dir(p), name)
		w, e := z.Create(rel)
		if e != nil {
			return e
		}
		f, e := os.Open(name)
		if e != nil {
			return e
		}
		defer f.Close()
		_, e = io.Copy(w, f)
		return e
	})
}
