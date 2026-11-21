// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

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
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	appExecLinkTag      = 0x8000001B
	trustedInstallerSID = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
)

type wingetBackend struct {
	path           string
	avail          bool
	reason         string
	tempRoot       string
	dataDir        string
	sourceVerified bool
	sourceCheckErr error
}

func newBackendWithDataDir(dataDir string) Backend {
	tempRoot := ""
	if dataDir != "" {
		tempRoot = filepath.Join(dataDir, "tmp")
	} else if pd := os.Getenv("ProgramData"); pd != "" {
		tempRoot = filepath.Join(pd, "Lanbay", "tmp")
	} else {
		return NewDisabledBackend("directorio temporal seguro no disponible")
	}
	b := &wingetBackend{tempRoot: tempRoot, dataDir: dataDir}
	b.detect()
	return b
}

func (b *wingetBackend) Available() bool { return b.avail }
func (b *wingetBackend) Reason() string  { return b.reason }

// SourceVerified reports whether the SW-3 source check succeeded.
func (b *wingetBackend) SourceVerified() bool { return b.sourceVerified }

// SourceCheckErr returns the last SW-3 verification error, if any.
func (b *wingetBackend) SourceCheckErr() error { return b.sourceCheckErr }

// verifySource runs `winget source export` and `winget settings export` and
// validates them per SW-3. Returns (true, err) when a confirmed violation is
// detected, (false, err) when the check could not be performed.
func (b *wingetBackend) verifySource(ctx context.Context) (bool, error) {

	jobTemp, err := b.newJobTemp()
	if err != nil {
		return false, fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(jobTemp)

	// source export
	sctx, scancel := context.WithTimeout(ctx, 1*time.Minute)
	defer scancel()
	_, out, err := runCommand(sctx, b.path, []string{"source", "export", "winget", "--disable-interactivity"}, b.env(jobTemp), jobTemp, io.Discard)
	if err != nil && len(out) == 0 {
		return false, fmt.Errorf("winget source export: %w", err)
	}
	if err := VerifyWingetSourceExport(string(out)); err != nil {
		// If winget produced output we could parse, it's a violation.
		if len(out) > 0 {
			return true, err
		}
		return false, err
	}

	// settings export
	stctx, stcancel := context.WithTimeout(ctx, 1*time.Minute)
	defer stcancel()
	_, sout, serr := runCommand(stctx, b.path, []string{"settings", "export", "--disable-interactivity"}, b.env(jobTemp), jobTemp, io.Discard)
	if serr != nil && len(sout) == 0 {
		// settings export unsupported -> unverifiable, not a violation
		return false, fmt.Errorf("winget settings export: %w", serr)
	}
	if len(sout) > 0 {
		if err := VerifyWingetSettingsExport(string(sout)); err != nil {
			return true, err
		}
	}
	return false, nil
}

func (b *wingetBackend) detect() {
	path, err := resolveWingetPath()
	if err != nil {
		b.avail = false
		b.reason = "winget no disponible o no verificado: " + err.Error()
		return
	}
	b.path = path
	// SW-3: verify the winget source URL and that InstallerHashOverride /
	// LocalManifestFiles are disabled. Fail closed on a confirmed violation.
	violation, verr := b.verifySource(context.Background())
	if verr != nil {
		b.sourceCheckErr = verr
		if violation {
			b.avail = false
			b.reason = "winget no verificado (SW-3): " + verr.Error()
			return
		}
		// Unverifiable (e.g. older winget): keep available but record.
	}
	b.sourceVerified = (verr == nil)
	b.avail = true
	b.reason = ""
}

func resolveWingetPath() (string, error) {
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		return "", errors.New("LOCALAPPDATA no disponible")
	}
	alias := filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe")
	tag, pkg, err := readAppExecLink(alias)
	if err != nil {
		return "", fmt.Errorf("leer reparse point del alias: %w", err)
	}
	if tag != appExecLinkTag {
		return "", fmt.Errorf("reparse tag inesperado: 0x%X", tag)
	}
	if !strings.HasPrefix(pkg, "Microsoft.DesktopAppInstaller_") || !strings.HasSuffix(pkg, "_8wekyb3d8bbwe") {
		return "", fmt.Errorf("package family inesperado: %q", pkg)
	}

	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	pattern := filepath.Join(programFiles, "WindowsApps", "Microsoft.DesktopAppInstaller_*_8wekyb3d8bbwe", "winget.exe")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return "", errors.New("no se encontró winget.exe real en WindowsApps")
	}
	sort.Slice(matches, func(i, j int) bool {
		return versionLess(matches[i], matches[j])
	})
	real := matches[len(matches)-1]
	root := filepath.Join(programFiles, "WindowsApps")
	rel, err := filepath.Rel(root, real)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("ruta real fuera de WindowsApps")
	}
	if _, err := os.Stat(real); err != nil {
		return "", err
	}
	if err := checkTrustedInstallerOwner(real); err != nil {
		return "", err
	}
	return real, nil
}

func readAppExecLink(path string) (uint32, string, error) {
	fd, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, "", err
	}
	defer windows.CloseHandle(fd)
	buf := make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var returned uint32
	if err := windows.DeviceIoControl(fd, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &buf[0], uint32(len(buf)), &returned, nil); err != nil {
		return 0, "", err
	}
	if returned < 12 {
		return 0, "", errors.New("reparse data demasiado corto")
	}
	tag := *(*uint32)(unsafe.Pointer(&buf[0]))
	// AppExecLinkReparseBuffer: Version (4 bytes) at offset 8, then StringList.
	off := 12
	var parts []string
	for off < int(returned) {
		start := off
		for off+1 < int(returned) {
			if buf[off] == 0 && buf[off+1] == 0 {
				break
			}
			off += 2
		}
		if off >= int(returned) || off == start {
			off += 2
			continue
		}
		s := windows.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(&buf[start]))[:(off-start)/2])
		parts = append(parts, s)
		off += 2
	}
	pkg := ""
	if len(parts) > 0 {
		pkg = parts[0]
	}
	return tag, pkg, nil
}

func versionLess(a, b string) bool {
	// Extract a version-like token from the path and compare numerically.
	va := versionToken(a)
	vb := versionToken(b)
	pa := splitVersion(va)
	pb := splitVersion(vb)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = atoi(pa[i])
		}
		if i < len(pb) {
			y = atoi(pb[i])
		}
		if x != y {
			return x < y
		}
	}
	return a < b
}

func versionToken(p string) string {
	// find Microsoft.DesktopAppInstaller_<ver>_8wekyb3d8bbwe
	for _, part := range strings.Split(p, string(os.PathSeparator)) {
		if strings.HasPrefix(part, "Microsoft.DesktopAppInstaller_") {
			fields := strings.Split(part, "_")
			if len(fields) >= 2 {
				return fields[1]
			}
		}
	}
	return p
}

func splitVersion(v string) []string {
	return strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func checkTrustedInstallerOwner(path string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner.String() != trustedInstallerSID {
		return fmt.Errorf("propietario no es TrustedInstaller: %s", owner.String())
	}
	return nil
}

func (b *wingetBackend) newJobTemp() (string, error) {
	if err := os.MkdirAll(b.tempRoot, 0700); err != nil {
		return "", err
	}
	return os.MkdirTemp(b.tempRoot, "job-")
}

func (b *wingetBackend) Search(ctx context.Context, query string) ([]SearchResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !b.avail {
		return nil, ErrUnavailable
	}
	jobTemp, err := b.newJobTemp()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(jobTemp)
	code, out, err := runCommand(ctx, b.path, []string{"search", "--query", query, "--source", "winget", "--accept-source-agreements", "--disable-interactivity"}, b.env(jobTemp), jobTemp, io.Discard)
	_ = code
	if err != nil && len(out) == 0 {
		return nil, err
	}
	res, perr := ParseSearch(string(out))
	if perr != nil {
		return nil, perr
	}
	for _, r := range res {
		if err := ValidateResultSource(r.Source); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (b *wingetBackend) IsInstalled(ctx context.Context, id string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !b.avail {
		return false, ErrUnavailable
	}
	jobTemp, err := b.newJobTemp()
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(jobTemp)
	code, out, err := runCommand(ctx, b.path, []string{"list", "--id", id, "--exact", "--accept-source-agreements", "--disable-interactivity"}, b.env(jobTemp), jobTemp, io.Discard)
	if err != nil {
		return false, err
	}
	return DecideInstalled(code, string(out), id)
}

func (b *wingetBackend) Install(ctx context.Context, id string, out io.Writer) (int, error) {
	if !b.avail {
		return 0, ErrUnavailable
	}
	jobTemp, err := b.newJobTemp()
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(jobTemp)
	code, _, err := runCommand(ctx, b.path, []string{
		"install", "--id", id, "--exact", "--source", "winget",
		"--silent", "--accept-package-agreements", "--accept-source-agreements",
		"--disable-interactivity",
	}, b.env(jobTemp), jobTemp, out)
	return code, err
}

func (b *wingetBackend) Upgrade(ctx context.Context, id string, out io.Writer) (int, error) {
	if !b.avail {
		return 0, ErrUnavailable
	}
	jobTemp, err := b.newJobTemp()
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(jobTemp)
	code, _, err := runCommand(ctx, b.path, []string{
		"upgrade", "--id", id, "--exact", "--source", "winget",
		"--silent", "--accept-package-agreements", "--accept-source-agreements",
		"--disable-interactivity",
	}, b.env(jobTemp), jobTemp, out)
	return code, err
}

func (b *wingetBackend) Uninstall(ctx context.Context, id string, out io.Writer) (int, error) {
	if !b.avail {
		return 0, ErrUnavailable
	}
	jobTemp, err := b.newJobTemp()
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(jobTemp)
	code, _, err := runCommand(ctx, b.path, []string{
		"uninstall", "--id", id, "--exact", "--silent",
		"--accept-source-agreements", "--disable-interactivity",
	}, b.env(jobTemp), jobTemp, out)
	return code, err
}

func (b *wingetBackend) ListUpgrades(ctx context.Context) ([]Update, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if !b.avail {
		return nil, ErrUnavailable
	}
	jobTemp, err := b.newJobTemp()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(jobTemp)
	_, out, err := runCommand(ctx, b.path, []string{"upgrade", "--accept-source-agreements", "--disable-interactivity"}, b.env(jobTemp), jobTemp, io.Discard)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	upd, err := ParseUpgrade(string(out))
	if err != nil {
		return nil, err
	}
	for _, u := range upd {
		if err := ValidateResultSource(u.Source); err != nil {
			return nil, err
		}
	}
	return upd, nil
}

func (b *wingetBackend) env(tempDir string) []string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	path := filepath.Join(systemRoot, "System32") + ";" + systemRoot
	return []string{
		"PATH=" + path,
		"SystemRoot=" + systemRoot,
		"TEMP=" + tempDir,
		"TMP=" + tempDir,
	}
}
