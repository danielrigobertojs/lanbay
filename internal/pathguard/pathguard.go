// SPDX-License-Identifier: Apache-2.0
// Package pathguard is the only boundary between client paths and the disk.
package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid   = errors.New("invalid_path")
	ErrOutside   = errors.New("outside_allowed_volume")
	ErrProtected = errors.New("protected_zone")
	ErrRoot      = errors.New("volume_root")
)

type Guard struct{ Roots, Protected []string }

// Resolved is an authorized, opened filesystem object. Its fields are private
// so only pathguard can construct it from a client-supplied path.
type Resolved struct {
	path string
	file *os.File
}

func (r *Resolved) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

// File returns the already-open validated handle. It is for the internal
// filesystem boundary; callers must not close it separately.
func (r *Resolved) File() *os.File { return r.file }

// TakeFile transfers ownership of the validated handle to the caller. It is
// for downloads and other operations that must outlive Resolved itself.
func (r *Resolved) TakeFile() *os.File {
	if r == nil {
		return nil
	}
	f := r.file
	r.file = nil
	return f
}

// Path is canonical and intended only for display/logging, never for reopening
// a mutating client operation.
func (r *Resolved) Path() string { return r.path }

// Resolve validates client and opens its final object before returning it.
func (g Guard) Resolve(client string, write bool) (*Resolved, error) {
	if err := lexical(client); err != nil {
		return nil, err
	}
	r, err := resolveOpen(client, write)
	if err != nil {
		return nil, fmt.Errorf("resolve existing path: %w", err)
	}
	if _, err := g.authorize(r.path, write); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

// ResolveParent opens an existing directory for a child mutation. Unlike
// Resolve(..., true), an allowed volume root is valid here because the root
// itself is not being renamed or deleted.
func (g Guard) ResolveParent(client string) (*Resolved, error) {
	if err := lexical(client); err != nil {
		return nil, err
	}
	// Parent handles are only used as RootDirectory handles for relative
	// operations. They require traversal/metadata access, not DELETE access.
	r, err := resolveOpen(client, false)
	if err != nil {
		return nil, fmt.Errorf("resolve parent: %w", err)
	}
	if _, err := g.authorizeParent(r.path); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

func (g Guard) Check(client string, write bool) (string, error) {
	r, err := g.Resolve(client, write)
	if err != nil {
		return "", err
	}
	defer r.Close()
	return r.path, nil
}

// CheckNew validates an untrusted leaf below an existing, authorized parent.
func (g Guard) CheckNew(parent, name string, write bool) (string, error) {
	if err := lexicalName(name); err != nil {
		return "", err
	}
	p, err := g.Check(parent, false)
	if err != nil {
		return "", err
	}
	result, err := g.authorize(filepath.Join(p, name), write)
	if err != nil {
		return "", err
	}
	if !samePath(filepath.Dir(result), p) {
		return "", ErrInvalid
	}
	return result, nil
}

// CheckTransfer authorizes both ends of a copy or move. The source is checked
// as a write because a successful move removes it, and a copy must not be used
// to exfiltrate a protected zone. Callers must use CheckNew for a destination
// leaf that does not exist yet.
func (g Guard) CheckTransfer(source, destination string) (string, string, error) {
	from, err := g.Check(source, true)
	if err != nil {
		return "", "", fmt.Errorf("authorize source: %w", err)
	}
	to, err := g.Check(destination, true)
	if err != nil {
		return "", "", fmt.Errorf("authorize destination: %w", err)
	}
	return from, to, nil
}

func (g Guard) authorize(path string, write bool) (string, error) {
	return g.authorizeWithRoot(path, write, false)
}

func (g Guard) authorizeParent(path string) (string, error) {
	p := filepath.Clean(path)
	root := ""
	for _, candidate := range g.Roots {
		canonical, err := resolve(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve allowed root: %w", err)
		}
		if within(p, canonical) {
			root = canonical
			break
		}
	}
	if root == "" {
		return "", ErrOutside
	}
	for _, protected := range g.Protected {
		canonical, err := resolve(protected)
		if err != nil {
			return "", fmt.Errorf("resolve protected zone: %w", err)
		}
		if within(p, canonical) {
			return "", ErrProtected
		}
	}
	return p, nil
}

func (g Guard) authorizeWithRoot(path string, write, allowRoot bool) (string, error) {
	p := filepath.Clean(path)
	root := ""
	for _, candidate := range g.Roots {
		canonical, err := resolve(candidate)
		if err != nil {
			return "", fmt.Errorf("resolve allowed root: %w", err)
		}
		if within(p, canonical) {
			root = canonical
			break
		}
	}
	if root == "" {
		return "", ErrOutside
	}
	if write && !allowRoot && samePath(p, root) {
		return "", ErrRoot
	}
	if write {
		for _, protected := range g.Protected {
			canonical, err := resolve(protected)
			if err != nil {
				return "", fmt.Errorf("resolve protected zone: %w", err)
			}
			if within(p, canonical) || within(canonical, p) {
				return "", ErrProtected
			}
		}
	}
	return p, nil
}

func lexical(p string) error {
	if p == "" || len(p) > 32767 || !utf8.ValidString(p) {
		return ErrInvalid
	}
	q := strings.ReplaceAll(p, "\\", "/")
	lower := strings.ToLower(q)
	if strings.HasPrefix(q, "//") || strings.HasPrefix(lower, "/?/unc/") || strings.HasPrefix(lower, "/./") || strings.HasPrefix(lower, "/?/globalroot") || strings.HasPrefix(lower, "/??/") || strings.Contains(lower, "volume{") || !absolute(q) {
		return ErrInvalid
	}
	parts := strings.Split(q, "/")
	for i, part := range parts {
		if i == 0 && len(part) == 2 && part[1] == ':' {
			continue
		}
		if err := lexicalName(part); err != nil {
			return err
		}
	}
	if len(q) > 2 && strings.Contains(q[2:], ":") {
		return ErrInvalid
	}
	return nil
}
func absolute(p string) bool {
	return strings.HasPrefix(p, "/") || (len(p) >= 3 && p[1] == ':' && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[2] == '/')
}
func lexicalName(part string) error {
	if part == "" {
		return nil
	}
	if part == "." || part == ".." || strings.ContainsAny(part, `\\/`) || strings.TrimRight(part, ". ") != part || reserved(part) {
		return ErrInvalid
	}
	for _, r := range part {
		if r < 0x20 || strings.ContainsRune(`<>:"|?*`, r) {
			return ErrInvalid
		}
	}
	return nil
}
func reserved(s string) bool {
	s = strings.ToUpper(strings.TrimRight(strings.Split(s, ".")[0], " ."))
	if s == "CON" || s == "PRN" || s == "AUX" || s == "NUL" || s == "CONIN$" || s == "CONOUT$" {
		return true
	}
	if len(s) == 4 && (strings.HasPrefix(s, "COM") || strings.HasPrefix(s, "LPT")) {
		return s[3] >= '0' && s[3] <= '9'
	}
	if len([]rune(s)) == 4 && (strings.HasPrefix(s, "COM") || strings.HasPrefix(s, "LPT")) {
		return strings.ContainsRune("¹²³", []rune(s)[3])
	}
	return false
}
func within(path, base string) bool {
	p, b := components(path), components(base)
	if len(p) < len(b) {
		return false
	}
	for i := range b {
		if !strings.EqualFold(p[i], b[i]) {
			return false
		}
	}
	return true
}
func samePath(a, b string) bool {
	pa, pb := components(a), components(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if !strings.EqualFold(pa[i], pb[i]) {
			return false
		}
	}
	return true
}
func components(path string) []string {
	p := strings.Trim(strings.ReplaceAll(filepath.Clean(path), "\\", "/"), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
