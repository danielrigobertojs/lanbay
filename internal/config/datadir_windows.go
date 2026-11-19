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
	existed := false
	if _, err := os.Stat(path); err == nil {
		existed = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	if existed {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
		if err != nil {
			return err
		}
		owner, _, err := sd.Owner()
		if err != nil {
			return err
		}
		if !owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			return os.ErrPermission
		}
	}
	if err := protectPath(path, true); err != nil {
		return err
	}
	return filepath.WalkDir(path, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if file == path {
			return nil
		}
		return protectPath(file, entry.IsDir())
	})
}

// ProtectFile assigns the same explicit Administrators/SYSTEM-only DACL to a secret file.
func ProtectFile(path string) error { return protectPath(path, false) }

func protectPath(path string, inherit bool) error {
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	inheritance := uint32(windows.NO_INHERITANCE)
	if inherit {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Inheritance: inheritance, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_GROUP, TrusteeValue: windows.TrusteeValueFromSID(admins)}},
		{AccessPermissions: windows.GENERIC_ALL, AccessMode: windows.GRANT_ACCESS, Inheritance: inheritance, Trustee: windows.TRUSTEE{TrusteeForm: windows.TRUSTEE_IS_SID, TrusteeType: windows.TRUSTEE_IS_USER, TrusteeValue: windows.TrusteeValueFromSID(system)}},
	}, nil)
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, admins, nil, acl, nil); err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION)
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
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || dacl.AceCount != 2 || !owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		return os.ErrPermission
	}
	return nil
}
