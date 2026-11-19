// SPDX-License-Identifier: Apache-2.0
// Package pathguard is the only boundary between client paths and the disk.
package pathguard

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var (
	ErrInvalid   = errors.New("ruta no válida")
	ErrOutside   = errors.New("ruta fuera de los volúmenes permitidos")
	ErrProtected = errors.New("zona protegida: solo lectura")
	ErrRoot      = errors.New("no se puede modificar la raíz de un volumen")
)

// Guard resolves paths before authorizing them. Roots and protected paths must
// already be canonical absolute paths supplied by the host platform.
type Guard struct{ Roots, Protected []string }

// Check validates an untrusted path and authorizes a read or write operation.
func (g Guard) Check(client string, write bool) (string, error) {
	if err := lexical(client); err != nil {
		return "", err
	}
	p, err := resolve(client)
	if err != nil {
		return "", fmt.Errorf("resolver ruta: %w", err)
	}
	p = filepath.Clean(p)
	root := ""
	for _, candidate := range g.Roots {
		canonical, e := resolve(candidate)
		if e != nil {
			continue
		}
		if within(p, canonical) {
			root = filepath.Clean(canonical)
			break
		}
	}
	if root == "" {
		return "", ErrOutside
	}
	if write && samePath(p, root) {
		return "", ErrRoot
	}
	if write {
		for _, protected := range g.Protected {
			canonical, e := resolve(protected)
			if e == nil && within(p, canonical) {
				return "", ErrProtected
			}
		}
	}
	return p, nil
}

func lexical(p string) error {
	if p == "" || strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") || strings.HasPrefix(strings.ToLower(p), `\\?\unc\`) || strings.HasPrefix(p, `\\.\`) || strings.HasPrefix(strings.ToLower(p), `\\?\globalroot`) {
		return ErrInvalid
	}
	if strings.Contains(strings.ToLower(p), "::$data") || strings.Contains(p, ":$") {
		return ErrInvalid
	}
	for _, part := range strings.FieldsFunc(strings.ReplaceAll(p, "\\", "/"), func(r rune) bool { return r == '/' }) {
		trimmed := strings.TrimRight(part, ". ")
		if part == ".." || part == "." || trimmed != part || reserved(trimmed) {
			return ErrInvalid
		}
	}
	if !filepath.IsAbs(p) {
		return ErrInvalid
	}
	return nil
}
func reserved(s string) bool {
	s = strings.ToUpper(strings.Split(s, ".")[0])
	if s == "CON" || s == "PRN" || s == "AUX" || s == "NUL" {
		return true
	}
	if len(s) == 4 && (strings.HasPrefix(s, "COM") || strings.HasPrefix(s, "LPT")) && s[3] >= '1' && s[3] <= '9' {
		return true
	}
	return false
}
func within(path, base string) bool {
	p, b := filepath.Clean(path), filepath.Clean(base)
	if samePath(p, b) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(p), strings.ToLower(b)+string(filepath.Separator))
}
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
