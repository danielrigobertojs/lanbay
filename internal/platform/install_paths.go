// SPDX-License-Identifier: Apache-2.0
package platform

import (
	"fmt"
	"path/filepath"
	"strings"
)

// InstallBinDir returns the directory under Program Files where Lanbay installs
// its own executable so the elevated scheduled task cannot be replaced by an
// unprivileged process.
func InstallBinDir(programFiles string) string {
	return filepath.Join(programFiles, "Lanbay")
}

// InstallBinPath returns the full path of the executable installed for the task.
func InstallBinPath(programFiles string) string {
	return filepath.Join(InstallBinDir(programFiles), "lanbay.exe")
}

// IsTrustedTrustee reports whether the given SID or account name is allowed to
// have write access to the installed binary or its directory.
func IsTrustedTrustee(sidOrName string) bool {
	switch sidOrName {
	case "S-1-5-32-544", "S-1-5-18", "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464",
		"Administrators", "SYSTEM", "TrustedInstaller",
		"BUILTIN\\Administrators", "NT AUTHORITY\\SYSTEM", "NT SERVICE\\TrustedInstaller":
		return true
	}
	return false
}

// HasUntrustedWriter reports whether any of the supplied trustees (those with
// write access) is not a trusted installer principal.
func HasUntrustedWriter(trustees []string) bool {
	for _, trustee := range trustees {
		if !IsTrustedTrustee(trustee) {
			return true
		}
	}
	return false
}

// ACLEntry is a minimal description of an access-control entry used to test
// the ACL logic without Windows.
type ACLEntry struct {
	Trustee     string
	Mask        uint32
	InheritOnly bool
	Allowed     bool
}

// HasWriteMask reports whether mask grants any real write permission. It
// deliberately excludes READ_CONTROL and SYNCHRONIZE, which are present in
// every read ACE and must not be treated as write access.
func HasWriteMask(mask uint32) bool {
	const writeBits = 0x00000002 | // FILE_WRITE_DATA / FILE_ADD_FILE
		0x00000004 | // FILE_APPEND_DATA / FILE_ADD_SUBDIRECTORY
		0x00000010 | // FILE_WRITE_EA
		0x00000040 | // FILE_DELETE_CHILD
		0x00000100 | // FILE_WRITE_ATTRIBUTES
		0x00010000 | // DELETE
		0x00040000 | // WRITE_DAC
		0x00080000 | // WRITE_OWNER
		0x10000000 | // GENERIC_ALL
		0x40000000 // GENERIC_WRITE
	return mask&writeBits != 0
}

// UntrustedWritersFromEntries returns the trustees (with write access) that are
// not trusted installer principals.
func UntrustedWritersFromEntries(entries []ACLEntry) []string {
	seen := map[string]bool{}
	var writers []string
	for _, e := range entries {
		if !e.Allowed {
			continue
		}
		if e.InheritOnly {
			continue
		}
		if !HasWriteMask(e.Mask) {
			continue
		}
		if IsTrustedTrustee(e.Trustee) {
			continue
		}
		if !seen[e.Trustee] {
			seen[e.Trustee] = true
			writers = append(writers, e.Trustee)
		}
	}
	return writers
}

// SchtasksEndArgs builds the command that stops a running scheduled task.
func SchtasksEndArgs() []string {
	return []string{"schtasks.exe", "/End", "/TN", "Lanbay"}
}

// SchtasksDeleteArgs builds the command that deletes the scheduled task.
func SchtasksDeleteArgs() []string {
	return []string{"schtasks.exe", "/Delete", "/TN", "Lanbay", "/F"}
}

// TaskkillOtherInstancesArgs builds the command that kills every lanbay.exe
// except the one with the given PID (the uninstall process itself).
func TaskkillOtherInstancesArgs(pid int) []string {
	return []string{"taskkill.exe", "/F", "/IM", "lanbay.exe", "/FI", fmt.Sprintf("PID ne %d", pid)}
}

// FirewallRemoveArgs builds the command that removes the Lanbay firewall rule.
func FirewallRemoveArgs() []string {
	return []string{"netsh", "advfirewall", "firewall", "delete", "rule", "name=Lanbay HTTPS"}
}

// StopRunningInstanceCommands returns the sequence of commands executed by
// StopRunningInstance, in order.
func StopRunningInstanceCommands(pid int) [][]string {
	return [][]string{
		SchtasksEndArgs(),
		TaskkillOtherInstancesArgs(pid),
	}
}

// IsUnderInstallDir reports whether exePath is inside the Lanbay Program Files
// directory and therefore cannot be deleted while running.
func IsUnderInstallDir(exePath, programFiles string) bool {
	return strings.HasPrefix(strings.ToLower(exePath), strings.ToLower(InstallBinDir(programFiles))+`\`)
}
