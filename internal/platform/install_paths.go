// SPDX-License-Identifier: Apache-2.0
package platform

import "path/filepath"

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
