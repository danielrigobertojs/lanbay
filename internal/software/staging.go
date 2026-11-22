// SPDX-License-Identifier: Apache-2.0
package software

import (
	"bytes"
	"context"
	"crypto/rand"
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

// StageUploadedFile writes a validated installer to staging under a server-generated
// name. It checks the extension and the magic bytes (MZ for .exe, OLE CFB for .msi).
func StageUploadedFile(stagingDir, originalName string, data []byte) (string, error) {
	return StageInstallerStream(stagingDir, originalName, bytes.NewReader(data))
}

// installerEnv returns an explicit, minimal environment for elevated installers.
func installerEnv(tempDir string) []string {
	sysDir := systemDirectory()
	systemRoot := filepath.Dir(sysDir)
	return []string{
		"PATH=" + sysDir + ";" + systemRoot,
		"SystemRoot=" + systemRoot,
		"TEMP=" + tempDir,
		"TMP=" + tempDir,
	}
}

// MsiexecPath returns the absolute path to msiexec.
func MsiexecPath() string {
	dir := systemDirectory()
	p := filepath.Join(dir, "msiexec.exe")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	// No fallback: fail closed.
	return ""
}

// JobTempDir creates and returns a per-job temporary directory under DataDir.
func JobTempDir(dataDir, jobID string) (string, error) {
	dir := filepath.Join(dataDir, "tmp", jobID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return dir, nil
}

// RemoveAllPath removes a path (best effort).
func RemoveAllPath(path string) error {
	return os.RemoveAll(path)
}

// StageInstallerStream streams a validated installer to staging.
func StageInstallerStream(stagingDir, originalName string, r interface{ Read([]byte) (int, error) }) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalName))
	if ext != ".msi" && ext != ".exe" {
		return "", errors.New("extensión no permitida")
	}
	if err := EnsureStagingDir(stagingDir); err != nil {
		return "", err
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", err
	}
	id := hex.EncodeToString(idBytes)
	partPath := filepath.Join(stagingDir, id+ext+".part")
	f, err := os.OpenFile(partPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	header := make([]byte, 8)
	n, err := io.ReadFull(r, header)
	if err != nil || n < 8 {
		f.Close()
		_ = os.Remove(partPath)
		return "", errors.New("archivo demasiado pequeño")
	}
	if ext == ".exe" {
		if header[0] != 0x4D || header[1] != 0x5A {
			f.Close()
			_ = os.Remove(partPath)
			return "", errors.New("firma MZ no válida")
		}
	} else {
		magic := []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
		for i := range magic {
			if header[i] != magic[i] {
				f.Close()
				_ = os.Remove(partPath)
				return "", errors.New("firma OLE CFB no válida")
			}
		}
	}
	h := sha256.New()
	mw := io.MultiWriter(f, h)
	if _, err := mw.Write(header); err != nil {
		f.Close()
		_ = os.Remove(partPath)
		return "", err
	}
	if _, err := io.Copy(mw, r); err != nil {
		f.Close()
		_ = os.Remove(partPath)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(partPath)
		return "", err
	}
	finalName := id + ext
	if err := os.Rename(partPath, filepath.Join(stagingDir, finalName)); err != nil {
		_ = os.Remove(partPath)
		return "", err
	}
	return finalName, nil
}
