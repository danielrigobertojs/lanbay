// SPDX-License-Identifier: Apache-2.0
// Package files implements guarded file operations for the HTTP API.
package files

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

var (
	ErrRecycleUnavailable = errors.New("recycle_unavailable")
	ErrRecursiveDelete    = errors.New("recursive_delete_not_supported")
	ErrDirectoryCopy      = errors.New("directory_copy_not_supported")
)

type Trash interface {
	Delete(context.Context, string) error
}
type DeleteOptions struct {
	Permanent bool
	Recycle   bool
}

type Entry struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size"`
	Modified  time.Time `json:"modified"`
	Hidden    bool      `json:"hidden"`
	System    bool      `json:"system"`
}
type ListOptions struct {
	Page, PerPage  int
	Limit          int
	Sort           string
	Hidden, System bool
}
type Listing struct {
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
	Page    int     `json:"page"`
	PerPage int     `json:"per_page"`
}
type Manager struct {
	Guard pathguard.Guard
	Trash Trash
}
type Service = Manager

func (m Manager) List(ctx context.Context, client string, opt ListOptions) (Listing, error) {
	r, err := m.Guard.Resolve(client, false)
	if err != nil {
		return Listing{}, err
	}
	defer r.Close()
	p := r.Path()
	dirs, err := r.File().ReadDir(-1)
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
		path := filepath.Join(p, d.Name())
		system, e := isSystem(path)
		if e != nil {
			continue // The entry changed while listing.
		}
		if !opt.System && system {
			continue
		}
		items = append(items, Entry{Name: d.Name(), Path: path, Directory: d.IsDir(), Size: info.Size(), Modified: info.ModTime(), Hidden: strings.HasPrefix(d.Name(), "."), System: system})
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
	if opt.Limit > 0 {
		opt.PerPage = opt.Limit
	}
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
func (m Manager) Mkdir(ctx context.Context, parent, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return "", err
	}
	r, err := m.Guard.ResolveParent(parent)
	if err != nil {
		return "", err
	}
	defer r.Close()
	return p, mkdirAt(r, name, 0750)
}
func (m Manager) Rename(ctx context.Context, source string, args ...string) (string, error) {
	if len(args) < 1 || len(args) > 2 {
		return "", fmt.Errorf("rename requires name or parent and name")
	}
	parent, name := filepath.Dir(source), args[0]
	if len(args) == 2 {
		parent, name = args[0], args[1]
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	src, err := m.Guard.Resolve(source, true)
	if err != nil {
		return "", err
	}
	dst, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return "", err
	}
	dstParent, err := m.Guard.ResolveParent(parent)
	if err != nil {
		return "", err
	}
	defer src.Close()
	defer dstParent.Close()
	return dst, renameAt(src, dstParent, name)
}
func (m Manager) Move(ctx context.Context, source, parent, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := m.Guard.Resolve(source, true)
	if err != nil {
		return err
	}
	_, err = m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return err
	}
	dstParent, err := m.Guard.ResolveParent(parent)
	if err != nil {
		return err
	}
	defer src.Close()
	defer dstParent.Close()
	return renameAt(src, dstParent, name)
}
func (m Manager) Copy(ctx context.Context, source, parent, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := m.Guard.Resolve(source, false)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err := src.File().Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ErrDirectoryCopy
	}
	if _, err := m.Guard.CheckNew(parent, name, true); err != nil {
		return err
	}
	dstParent, err := m.Guard.ResolveParent(parent)
	if err != nil {
		return err
	}
	defer dstParent.Close()
	return copyFileAt(src, dstParent, name, info.Mode())
}
func (m Manager) CopyFile(ctx context.Context, source, parent, name string) (string, error) {
	dst, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return "", err
	}
	return dst, m.Copy(ctx, source, parent, name)
}
func (m Manager) Delete(ctx context.Context, source string, options DeleteOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r, err := m.Guard.Resolve(source, true)
	if err != nil {
		return err
	}
	defer r.Close()
	if !options.Permanent {
		if !options.Recycle || m.Trash == nil {
			return ErrRecycleUnavailable
		}
		return m.Trash.Delete(ctx, r.Path())
	}
	return deleteAt(r)
}

// Open resolves a file for a download. Directories are rejected so callers
// must use Zip, keeping content-disposition policy at the HTTP boundary.
func (m Manager) Open(client string) (*os.File, os.FileInfo, error) {
	r, err := m.Guard.Resolve(client, false)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	f := r.File()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		_ = f.Close()
		if err == nil {
			err = fmt.Errorf("path is a directory")
		}
		return nil, nil, err
	}
	return r.TakeFile(), info, nil
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
