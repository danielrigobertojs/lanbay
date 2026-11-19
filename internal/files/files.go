// SPDX-License-Identifier: Apache-2.0
// Package files contains filesystem operations guarded by pathguard.
package files

import (
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

var ErrDirectoryCopy = errors.New("directory_copy_not_supported")

// Entry is the server-side representation used for a paginated directory.
type Entry struct {
	Name      string    `json:"name"`
	Directory bool      `json:"directory"`
	Size      int64     `json:"size"`
	Modified  time.Time `json:"modified"`
	Hidden    bool      `json:"hidden"`
}

type ListOptions struct {
	Offset     int
	Limit      int
	ShowHidden bool
	Sort       string
	Descending bool
}

type Page struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
	Total   int     `json:"total"`
}

// Service is deliberately the only filesystem API exposed to HTTP handlers.
// Every public method authorizes client-controlled paths through Guard.
type Service struct{ Guard pathguard.Guard }

func (s Service) List(ctx context.Context, clientPath string, options ListOptions) (Page, error) {
	path, err := s.Guard.Check(clientPath, false)
	if err != nil {
		return Page{}, fmt.Errorf("authorize directory: %w", err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return Page{}, fmt.Errorf("read directory: %w", err)
	}
	page := Page{Path: path}
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			return Page{}, err
		}
		hidden := strings.HasPrefix(item.Name(), ".")
		if hidden && !options.ShowHidden {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		page.Entries = append(page.Entries, Entry{Name: item.Name(), Directory: item.IsDir(), Size: info.Size(), Modified: info.ModTime(), Hidden: hidden})
	}
	sortEntries(page.Entries, options)
	page.Total = len(page.Entries)
	if options.Offset < 0 {
		options.Offset = 0
	}
	if options.Limit <= 0 || options.Limit > 1000 {
		options.Limit = 1000
	}
	if options.Offset >= len(page.Entries) {
		page.Entries = nil
		return page, nil
	}
	end := options.Offset + options.Limit
	if end > len(page.Entries) {
		end = len(page.Entries)
	}
	page.Entries = page.Entries[options.Offset:end]
	return page, nil
}

func (s Service) Mkdir(ctx context.Context, parent, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := s.Guard.CheckNew(parent, name, true)
	if err != nil {
		return "", fmt.Errorf("authorize new directory: %w", err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return path, nil
}

func (s Service) Rename(ctx context.Context, source, parent, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	from, err := s.Guard.Check(source, true)
	if err != nil {
		return "", fmt.Errorf("authorize source: %w", err)
	}
	to, err := s.Guard.CheckNew(parent, name, true)
	if err != nil {
		return "", fmt.Errorf("authorize destination: %w", err)
	}
	if err := os.Rename(from, to); err != nil {
		return "", fmt.Errorf("rename: %w", err)
	}
	return to, nil
}

func (s Service) CopyFile(ctx context.Context, source, parent, name string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	from, err := s.Guard.Check(source, false)
	if err != nil {
		return "", fmt.Errorf("authorize source: %w", err)
	}
	to, err := s.Guard.CheckNew(parent, name, true)
	if err != nil {
		return "", fmt.Errorf("authorize destination: %w", err)
	}
	info, err := os.Stat(from)
	if err != nil {
		return "", fmt.Errorf("stat source: %w", err)
	}
	if info.IsDir() {
		return "", ErrDirectoryCopy
	}
	in, err := os.Open(from)
	if err != nil {
		return "", fmt.Errorf("open source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return "", fmt.Errorf("create destination: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(to)
		return "", fmt.Errorf("copy file: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(to)
		return "", fmt.Errorf("close destination: %w", closeErr)
	}
	return to, nil
}

func sortEntries(entries []Entry, options ListOptions) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		var less bool
		switch options.Sort {
		case "size":
			less = a.Size < b.Size
		case "modified":
			less = a.Modified.Before(b.Modified)
		default:
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		if options.Descending {
			return !less
		}
		return less
	})
}

// Parent returns a client path suitable for use as a destination parent.
func Parent(path string) string { return filepath.Dir(path) }
