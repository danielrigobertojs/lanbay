// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

type wingetBackend struct {
	path    string
	avail   bool
	reason  string
	tempDir string
}

func newBackend() Backend {
	b := &wingetBackend{tempDir: os.TempDir()}
	b.detect()
	return b
}

func (b *wingetBackend) Available() bool { return b.avail }
func (b *wingetBackend) Reason() string  { return b.reason }

func (b *wingetBackend) detect() {
	localAppData := os.Getenv("LOCALAPPDATA")
	candidates := []string{}
	if localAppData != "" {
		candidates = append(candidates, filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe"))
	}
	candidates = append(candidates, "winget.exe")
	for _, c := range candidates {
		info, err := os.Lstat(c)
		if err != nil {
			continue
		}
		// Require a reparse point (App Installer alias). A plain executable is
		// treated as untrusted (threat model SW-1).
		if info.Mode()&os.ModeSymlink == 0 && (info.Sys() == nil) {
			// fall through to attribute check below
		}
		attrs, aerr := windows.GetFileAttributes(windows.StringToUTF16Ptr(c))
		if aerr != nil || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
			continue
		}
		b.path = c
		b.avail = true
		b.reason = ""
		return
	}
	b.avail = false
	b.reason = "winget no disponible o no verificado"
}

func (b *wingetBackend) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if !b.avail {
		return nil, ErrUnavailable
	}
	code, out, err := runCommand(ctx, b.path, []string{"search", "--query", query, "--source", "winget"}, b.env(), b.tempDir, io.Discard)
	_ = code
	if err != nil && len(out) == 0 {
		return nil, err
	}
	res, perr := ParseSearch(string(out))
	if perr != nil {
		return nil, perr
	}
	return res, nil
}

func (b *wingetBackend) IsInstalled(ctx context.Context, id string) (bool, error) {
	if !b.avail {
		return false, ErrUnavailable
	}
	_, out, err := runCommand(ctx, b.path, []string{"list", "--id", id, "--exact"}, b.env(), b.tempDir, io.Discard)
	if err != nil && len(out) == 0 {
		return false, err
	}
	res, perr := ParseList(string(out))
	if perr != nil {
		// Fail closed: an unrecognised listing is treated as installed so the
		// caller rejects the installation.
		return true, perr
	}
	for _, r := range res {
		if strings.EqualFold(r.ID, id) {
			return true, nil
		}
	}
	return false, nil
}

func (b *wingetBackend) Install(ctx context.Context, id string, out io.Writer) (int, error) {
	if !b.avail {
		return 0, ErrUnavailable
	}
	code, _, err := runCommand(ctx, b.path, []string{
		"install", "--id", id, "--exact", "--source", "winget",
		"--silent", "--accept-package-agreements", "--accept-source-agreements",
		"--disable-interactivity",
	}, b.env(), b.tempDir, out)
	return code, err
}

func (b *wingetBackend) Upgrade(ctx context.Context, id string, out io.Writer) (int, error) {
	if !b.avail {
		return 0, ErrUnavailable
	}
	code, _, err := runCommand(ctx, b.path, []string{
		"upgrade", "--id", id, "--exact", "--source", "winget",
		"--silent", "--accept-package-agreements", "--accept-source-agreements",
		"--disable-interactivity",
	}, b.env(), b.tempDir, out)
	return code, err
}

func (b *wingetBackend) Uninstall(ctx context.Context, id string, out io.Writer) (int, error) {
	if !b.avail {
		return 0, ErrUnavailable
	}
	code, _, err := runCommand(ctx, b.path, []string{
		"uninstall", "--id", id, "--exact", "--silent",
		"--accept-source-agreements", "--disable-interactivity",
	}, b.env(), b.tempDir, out)
	return code, err
}

func (b *wingetBackend) ListUpgrades(ctx context.Context) ([]Update, error) {
	if !b.avail {
		return nil, ErrUnavailable
	}
	_, out, err := runCommand(ctx, b.path, []string{"upgrade"}, b.env(), b.tempDir, io.Discard)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return ParseUpgrade(string(out))
}

func (b *wingetBackend) env() []string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	path := filepath.Join(systemRoot, "System32") + ";" + systemRoot
	return []string{
		"PATH=" + path,
		"SystemRoot=" + systemRoot,
		"TEMP=" + b.tempDir,
		"TMP=" + b.tempDir,
	}
}
