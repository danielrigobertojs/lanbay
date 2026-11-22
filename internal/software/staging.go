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
	"regexp"
	"runtime"
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

var stagingNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.(?i:msi|exe)$`)

// ResolveStagingPath accepts ONLY a base file name. It resolves the staging
// directory and verifies the resulting path stays inside it (handle-based on
// Windows, EvalSymlinks on Unix). Any separator, ADS, UNC or device prefix is
// rejected by the regex before touching the filesystem.
func ResolveStagingPath(stagingDir, name string) (string, error) {
	if strings.Contains(name, string(os.PathSeparator)) || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("path fuera de staging")
	}
	if !stagingNameRE.MatchString(name) {
		return "", errors.New("path fuera de staging")
	}
	root, err := finalPath(stagingDir)
	if err != nil {
		return "", err
	}
	if err := EnsureStagingDir(stagingDir); err != nil {
		return "", err
	}
	p := filepath.Join(stagingDir, name)
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return "", errors.New("path fuera de staging")
	}
	final, err := finalPath(p)
	if err != nil {
		return "", errors.New("path fuera de staging")
	}
	if !samePath(filepath.Dir(final), root) {
		return "", errors.New("path fuera de staging")
	}
	return p, nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
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
