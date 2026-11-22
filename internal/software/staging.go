// SPDX-License-Identifier: Apache-2.0
package software

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// EnsureStagingDir creates the staging directory with a restrictive mode.
func EnsureStagingDir(stagingDir string) error {
	return os.MkdirAll(stagingDir, 0700)
}

// RemoveStagingFile removes a file from staging (best effort).
func RemoveStagingFile(path string) error {
	return os.Remove(path)
}

// ResolveStagingPath validates that a client-supplied value resolves to a
// regular .msi/.exe file directly inside the staging directory. It accepts a
// bare file name or a path already contained in staging; anything else
// (traversal, symlinks/junctions, ADS, UNC, prefixes, the staging root
// itself, directories, other extensions) is rejected.
func ResolveStagingPath(stagingDir, input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", errors.New("path requerido")
	}
	if err := EnsureStagingDir(stagingDir); err != nil {
		return "", err
	}
	stagingAbs, err := filepath.Abs(stagingDir)
	if err != nil {
		return "", err
	}
	stagingResolved, err := filepath.EvalSymlinks(stagingAbs)
	if err != nil {
		stagingResolved = stagingAbs
	}
	// On Windows, EvalSymlinks may not resolve; fall back to finalPath.
	if rp, err := finalPath(stagingAbs); err == nil && rp != "" {
		stagingResolved = rp
	}

	candidate := input
	if !filepath.IsAbs(candidate) {
		if filepath.Base(candidate) == candidate {
			candidate = filepath.Join(stagingAbs, candidate)
		}
	}
	candAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	// Reject ADS, UNC and device prefixes outright.
	lower := strings.ToLower(candAbs)
	if strings.Contains(lower, "::") || strings.HasPrefix(lower, `\\?\`) || strings.HasPrefix(lower, `\\`) {
		return "", errors.New("path fuera de staging")
	}
	if fi, err := os.Lstat(candAbs); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("path fuera de staging")
		}
		if !fi.Mode().IsRegular() {
			return "", errors.New("path fuera de staging")
		}
		ext := strings.ToLower(filepath.Ext(candAbs))
		if ext != ".msi" && ext != ".exe" {
			return "", errors.New("extensión no permitida")
		}
	}
	resolved, err := finalPath(candAbs)
	if err != nil {
		return "", errors.New("path fuera de staging")
	}
	if resolved == "" {
		resolved = candAbs
	}
	// On Unix, ensure symlinks are fully resolved.
	if rp, err := filepath.EvalSymlinks(candAbs); err == nil {
		resolved = rp
	}
	rel, err := filepath.Rel(stagingResolved, resolved)
	if err != nil {
		return "", errors.New("path fuera de staging")
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", errors.New("path fuera de staging")
	}
	// Reject the staging root itself and any directory.
	if resolved == stagingResolved {
		return "", errors.New("path fuera de staging")
	}
	return resolved, nil
}

func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func InspectInstaller(path string) (InstallerInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return InstallerInfo{}, err
	}
	hash, err := SHA256File(path)
	if err != nil {
		return InstallerInfo{}, err
	}
	info := InstallerInfo{
		Path:   path,
		Name:   fi.Name(),
		Size:   fi.Size(),
		SHA256: hash,
		Type:   installerType(path),
	}
	ai, _ := VerifyAuthenticode(context.Background(), path)
	info.Authenticode = ai
	return info, nil
}

func installerType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".msi":
		return "msi"
	case ".exe":
		return "exe"
	default:
		return ext
	}
}

// StagingFileExists reports whether a path exists in staging.
func StagingFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
