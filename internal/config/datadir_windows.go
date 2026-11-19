// SPDX-License-Identifier: Apache-2.0
//go:build windows

package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const (
	fileAttributeReparsePoint = 0x0400
	fileShareRead             = 0x00000001
	fileShareWrite            = 0x00000002
	fileShareDelete           = 0x00000004
	openExisting              = 3
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
	if err := rejectUnsafeRoot(path); err != nil {
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
		if err := rejectUnsafeEntry(file, entry); err != nil {
			return err
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

// rejectUnsafeEntry prevents a privileged migration from following a junction,
// symlink, or hard link planted by an unprivileged user.
func rejectUnsafeEntry(path string, entry fs.DirEntry) error {
	if entry.Type()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		return fmt.Errorf("unsafe reparse entry: %s", path)
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(pointer)
	if err != nil {
		return err
	}
	if attrs&fileAttributeReparsePoint != 0 {
		return fmt.Errorf("unsafe reparse entry: %s", path)
	}
	if entry.IsDir() {
		return nil
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ, fileShareRead|fileShareWrite|fileShareDelete, nil, openExisting, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.NumberOfLinks > 1 {
		return fmt.Errorf("unsafe hard-linked entry: %s", path)
	}
	return nil
}

func rejectUnsafeRoot(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
		return fmt.Errorf("unsafe data directory: %s", path)
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(pointer)
	if err != nil {
		return err
	}
	if attrs&fileAttributeReparsePoint != 0 {
		return fmt.Errorf("unsafe data directory: %s", path)
	}
	return nil
}
