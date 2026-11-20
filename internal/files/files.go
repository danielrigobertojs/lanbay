// SPDX-License-Identifier: Apache-2.0
// Package files implements guarded file operations for the HTTP API.
package files

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

var (
	ErrRecycleUnavailable = errors.New("recycle_unavailable")
)

const (
	maxWalkDepth   = 64
	maxWalkEntries = 100000
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
	var dirs []os.DirEntry
	for {
		batch, err := r.File().ReadDir(256)
		dirs = append(dirs, batch...)
		if err != nil {
			if err != io.EOF {
				return Listing{}, fmt.Errorf("read directory: %w", err)
			}
			break
		}
		if len(batch) == 0 {
			break
		}
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
	src, err := m.Guard.ResolveWalk(source, false)
	if err != nil {
		return err
	}
	defer src.Close()
	destination, err := m.Guard.CheckNew(parent, name, true)
	if err != nil {
		return err
	}
	if isWithin(destination, src.Path()) {
		return fmt.Errorf("cannot copy a directory into itself")
	}
	dstParent, err := m.Guard.ResolveParent(parent)
	if err != nil {
		return err
	}
	defer dstParent.Close()
	info, err := src.File().Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := mkdirAt(dstParent, name, info.Mode()); err != nil {
			return err
		}
		dst, err := pathguard.OpenChild(dstParent, name, false, true)
		if err != nil {
			return err
		}
		defer dst.Close()
		return copyTree(ctx, src, dst, 0, newWalkCount())
	}
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
	info, err := r.File().Stat()
	if err != nil {
		_ = r.Close()
		return err
	}
	if info.IsDir() {
		_ = r.Close()
		r, err = m.Guard.ResolveWalk(source, true)
		if err != nil {
			return err
		}
	}
	defer r.Close()
	if !options.Permanent {
		if !options.Recycle || m.Trash == nil {
			return ErrRecycleUnavailable
		}
		return m.Trash.Delete(ctx, r.Path())
	}
	return deleteTree(ctx, r, 0, newWalkCount())
}

// Open resolves a file for a download. Directories are rejected so callers
// must use Zip, keeping content-disposition policy at the HTTP boundary.
func (m Manager) Open(client string) (*os.File, os.FileInfo, error) {
	r, err := m.Guard.ResolveWalk(client, false)
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
	if err := ctx.Err(); err != nil {
		return err
	}
	r, err := m.Guard.Resolve(client, false)
	if err != nil {
		return err
	}
	defer r.Close()
	info, err := r.File().Stat()
	if err != nil {
		return err
	}
	z := zip.NewWriter(out)
	defer z.Close()
	if info.IsDir() {
		omitted, err := zipTree(ctx, z, r, filepath.Base(r.Path()), 0, newWalkCount())
		if err != nil {
			return err
		}
		if len(omitted) > 0 {
			z.SetComment("Lanbay: omitted reparse points: " + strings.Join(omitted, ", "))
		}
		return nil
	}
	w, err := z.Create(filepath.Base(r.Path()))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, r.File())
	return err
}

type walkCount struct{ entries int }

func newWalkCount() *walkCount { return &walkCount{} }

func (c *walkCount) add(ctx context.Context, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > maxWalkDepth {
		return fmt.Errorf("directory walk depth exceeds %d", maxWalkDepth)
	}
	c.entries++
	if c.entries > maxWalkEntries {
		return fmt.Errorf("directory walk entries exceeds %d", maxWalkEntries)
	}
	return nil
}

// child opens a name relative to dir and deliberately refuses to traverse a
// reparse point. Copy and ZIP omit it; deletion deletes the link itself.
func child(ctx context.Context, dir *pathguard.Resolved, name string, write bool, depth int, count *walkCount) (*pathguard.Resolved, os.FileInfo, error) {
	if err := count.add(ctx, depth); err != nil {
		return nil, nil, err
	}
	r, err := pathguard.OpenChild(dir, name, write, false)
	if err != nil {
		return nil, nil, err
	}
	if r.Reparse() {
		return r, nil, nil
	}
	info, err := r.File().Stat()
	if err != nil {
		_ = r.Close()
		return nil, nil, err
	}
	return r, info, nil
}

func copyTree(ctx context.Context, source, destination *pathguard.Resolved, depth int, count *walkCount) error {
	for {
		entries, err := source.File().ReadDir(256)
		for _, entry := range entries {
			src, info, err := child(ctx, source, entry.Name(), false, depth+1, count)
			if err != nil {
				return err
			}
			if src.Reparse() {
				slog.Warn("omitting reparse point from copy", "path", src.Path())
				_ = src.Close()
				continue
			}
			if info.IsDir() {
				_ = src.Close()
				src, err = pathguard.OpenChild(source, entry.Name(), false, true)
				if err != nil {
					return err
				}
				if src.Reparse() {
					slog.Warn("omitting reparse point from copy", "path", src.Path())
					_ = src.Close()
					continue
				}
				if err = mkdirAt(destination, entry.Name(), info.Mode()); err != nil {
					_ = src.Close()
					return err
				}
				dst, err := pathguard.OpenChild(destination, entry.Name(), false, true)
				if err != nil {
					_ = src.Close()
					return err
				}
				err = copyTree(ctx, src, dst, depth+1, count)
				_ = dst.Close()
				_ = src.Close()
				if err != nil {
					return err
				}
				continue
			}
			err = copyFileAt(src, destination, entry.Name(), info.Mode())
			_ = src.Close()
			if err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read source directory: %w", err)
		}
		if len(entries) == 0 {
			break
		}
	}
	return nil
}

func deleteTree(ctx context.Context, node *pathguard.Resolved, depth int, count *walkCount) error {
	if err := count.add(ctx, depth); err != nil {
		return err
	}
	if node.Reparse() {
		return deleteAt(node)
	}
	info, err := node.File().Stat()
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return deleteAt(node)
	}
	for {
		entries, err := node.File().ReadDir(256)
		for _, entry := range entries {
			childNode, childInfo, err := child(ctx, node, entry.Name(), true, depth+1, count)
			if err != nil {
				return err
			}
			if childNode.Reparse() {
				err = deleteAt(childNode)
				_ = childNode.Close()
				if err != nil {
					return err
				}
				continue
			}
			if childInfo.IsDir() {
				_ = childNode.Close()
				childNode, err = pathguard.OpenChild(node, entry.Name(), true, true)
				if err != nil {
					return err
				}
				if childNode.Reparse() {
					err = deleteAt(childNode)
					_ = childNode.Close()
					if err != nil {
						return err
					}
					continue
				}
			}
			err = deleteTree(ctx, childNode, depth+1, count)
			_ = childNode.Close()
			if err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read directory for delete: %w", err)
		}
		if len(entries) == 0 {
			break
		}
	}
	return deleteAt(node)
}

func zipTree(ctx context.Context, z *zip.Writer, dir *pathguard.Resolved, prefix string, depth int, count *walkCount) ([]string, error) {
	var omitted []string
	for {
		entries, err := dir.File().ReadDir(256)
		for _, entry := range entries {
			node, info, err := child(ctx, dir, entry.Name(), false, depth+1, count)
			if err != nil {
				return omitted, err
			}
			name := filepath.ToSlash(filepath.Join(prefix, entry.Name()))
			if node.Reparse() {
				slog.Warn("omitting reparse point from zip", "path", node.Path())
				omitted = append(omitted, node.Path())
				_ = node.Close()
				continue
			}
			if info.IsDir() {
				_ = node.Close()
				node, err = pathguard.OpenChild(dir, entry.Name(), false, true)
				if err != nil {
					return omitted, err
				}
				if node.Reparse() {
					slog.Warn("omitting reparse point from zip", "path", node.Path())
					omitted = append(omitted, node.Path())
					_ = node.Close()
					continue
				}
				childOmitted, err := zipTree(ctx, z, node, name, depth+1, count)
				omitted = append(omitted, childOmitted...)
				_ = node.Close()
				if err != nil {
					return omitted, err
				}
				continue
			}
			w, err := z.Create(name)
			if err == nil {
				_, err = io.Copy(w, node.File())
			}
			_ = node.Close()
			if err != nil {
				return omitted, err
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return omitted, fmt.Errorf("read directory for zip: %w", err)
		}
		if len(entries) == 0 {
			break
		}
	}
	return omitted, nil
}

func isWithin(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
