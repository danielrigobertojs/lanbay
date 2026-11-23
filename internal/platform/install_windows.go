// SPDX-License-Identifier: Apache-2.0
//go:build windows

package platform

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const scheduledTaskName = "Lanbay"

// IsWindows reports whether this build targets Windows.
func IsWindows() bool { return true }

// ProgramFilesDir resolves %ProgramFiles% via the shell.
func ProgramFilesDir() (string, error) {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", fmt.Errorf("resolve Program Files: %w", err)
	}
	return dir, nil
}

// InstallBinary copies the running executable into %ProgramFiles%\Lanbay so the
// elevated scheduled task cannot be replaced by an unprivileged process. It
// aborts if an untrusted principal can write to the directory or the file.
func InstallBinary(srcExe string) (string, error) {
	pf, err := ProgramFilesDir()
	if err != nil {
		return "", err
	}
	dstDir := InstallBinDir(pf)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", fmt.Errorf("create install dir: %w", err)
	}
	if err := CheckInstallPathWritable(dstDir); err != nil {
		_ = os.RemoveAll(dstDir)
		return "", err
	}
	dst := InstallBinPath(pf)
	if srcInfo, err := os.Stat(srcExe); err == nil {
		if dstInfo, err := os.Stat(dst); err == nil && os.SameFile(srcInfo, dstInfo) {
			// Already installed at the destination; nothing to copy.
		} else {
			if err := copyFile(srcExe, dst); err != nil {
				_ = os.Remove(dst)
				return "", fmt.Errorf("copy binary: %w", err)
			}
		}
	} else {
		if err := copyFile(srcExe, dst); err != nil {
			_ = os.Remove(dst)
			return "", fmt.Errorf("copy binary: %w", err)
		}
	}
	if err := CheckInstallPathWritable(dst); err != nil {
		_ = os.Remove(dst)
		_ = os.RemoveAll(dstDir)
		return "", err
	}
	return dst, nil
}

// RemoveInstallDir deletes %ProgramFiles%\Lanbay during uninstall.
func RemoveInstallDir() error {
	pf, err := ProgramFilesDir()
	if err != nil {
		return err
	}
	return os.RemoveAll(InstallBinDir(pf))
}

// CheckInstallPathWritable rejects the path if any non-trusted principal has
// write access to it.
func CheckInstallPathWritable(path string) error {
	trustees, err := untrustedWriters(path)
	if err != nil {
		return err
	}
	if HasUntrustedWriter(trustees) {
		return fmt.Errorf("untrusted principal can write to %s: %s", path, strings.Join(trustees, ", "))
	}
	return nil
}

func untrustedWriters(path string) ([]string, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, fmt.Errorf("read security info %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, err
	}
	if dacl == nil {
		// A null DACL grants everyone full control.
		return []string{"Everyone"}, nil
	}
	var entries []ACLEntry
	for i := uint16(0); i < dacl.AceCount; i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, uint32(i), &ace); err != nil {
			return nil, err
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		sidStr := sid.String()
		if sidStr == "" {
			sidStr = "unknown"
		}
		entries = append(entries, ACLEntry{
			Trustee:     sidStr,
			Mask:        uint32(ace.Mask),
			InheritOnly: ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0,
			Allowed:     ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE,
		})
	}
	return UntrustedWritersFromEntries(entries), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// InstallScheduledTask registers the Lanbay elevated logon task per ADR-004.
// The executable path travels through the LANBAY_EXE environment variable, not
// through the script text.
func InstallScheduledTask(exePath string) error {
	script := scheduledTaskScript()
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), "LANBAY_EXE="+exePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("register scheduled task: %w: %s", err, string(out))
	}
	return nil
}

// UninstallScheduledTask stops and removes the Lanbay scheduled task if present.
func UninstallScheduledTask() error {
	for _, args := range [][]string{SchtasksEndArgs(), SchtasksDeleteArgs()} {
		cmd := exec.Command(args[0], args[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			if args[1] == "/Delete" && !containsNotFound(string(out)) {
				return fmt.Errorf("delete scheduled task: %w: %s", err, string(out))
			}
		}
	}
	return nil
}

// AddFirewallRule allows inbound TCP on the given port, Private profile only,
// from the local subnet.
func AddFirewallRule(port int) error {
	_ = RemoveFirewallRule()
	cmd := exec.Command("netsh", "advfirewall", "firewall", "add", "rule",
		"name=Lanbay HTTPS", "dir=in", "action=allow", "protocol=TCP",
		"localport="+strconv.Itoa(port), "profile=private", "remoteip=localsubnet", "enable=yes")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("add firewall rule: %w: %s", err, string(out))
	}
	return nil
}

// RemoveFirewallRule deletes the Lanbay firewall rule if present.
func RemoveFirewallRule() error {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "delete", "rule", "name=Lanbay HTTPS")
	_ = cmd.Run()
	return nil
}

// StopRunningInstance stops the scheduled task and any other lanbay.exe
// instances, without killing the uninstall process itself.
func StopRunningInstance() {
	for _, args := range StopRunningInstanceCommands(os.Getpid()) {
		cmd := exec.Command(args[0], args[1:]...)
		_ = cmd.Run()
	}
	// If uninstall runs from the installed binary, it cannot be deleted now;
	// mark it for deletion on the next reboot.
	if pf, err := ProgramFilesDir(); err == nil {
		if exe, err := os.Executable(); err == nil && IsUnderInstallDir(exe, pf) {
			if p, err := windows.UTF16PtrFromString(exe); err == nil {
				_ = windows.MoveFileEx(p, nil, windows.MOVEFILE_DELAY_UNTIL_REBOOT)
			}
		}
	}
}

func containsNotFound(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "cannot find") || strings.Contains(low, "does not exist") || strings.Contains(low, "no tasks")
}
