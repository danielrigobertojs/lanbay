// SPDX-License-Identifier: Apache-2.0
//go:build windows

package config

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func DefaultDataDir() string {
	dir, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		panic("resolve ProgramData: " + err.Error())
	}
	return filepath.Join(dir, "Lanbay")
}

func ensureDataDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return protectPath(path)
}

// ProtectFile assigns the same explicit Administrators/SYSTEM-only DACL to a secret file.
func ProtectFile(path string) error { return protectPath(path) }

func protectPath(path string) error {
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_GROUP, TrusteeValue: windows.TrusteeValueFromSID(admins)}},
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(system)}},
	}, nil)
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || dacl.AceCount != 2 {
		return os.ErrPermission
	}
	return nil
}
