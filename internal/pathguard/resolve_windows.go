// SPDX-License-Identifier: Apache-2.0
//go:build windows

package pathguard

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxReparseHops = 8

// Access rights not exposed by golang.org/x/sys/windows.
const (
	fileAddFile         = 0x00000002
	fileAddSubdirectory = 0x00000004
)

// resolveOpen retains the final handle while walking each child relative to an
// already-open parent. OBJ_DONT_REPARSE prevents a directory swapped after its
// inspection from being followed by the next open.
func resolveOpen(path string, write, container bool) (*Resolved, error) {
	finalAccess := uint32(windows.FILE_GENERIC_READ)
	if container {
		// A directory used as the parent of a mutation needs the rights to
		// list, create files/subdirectories and (for deletes within it) delete
		// children. It must NOT carry DELETE on the directory itself, which
		// would allow removing the container.
		finalAccess = windows.FILE_LIST_DIRECTORY | fileAddFile | fileAddSubdirectory | windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	} else if write {
		// Mutations are performed with a handle-specific operation below. Do
		// not ask for write-data: DELETE is sufficient for rename/delete, and
		// permits operations on a readonly file.
		finalAccess = windows.DELETE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	}
	current := filepath.Clean(path)
	for hops := 0; ; {
		volume := filepath.VolumeName(current)
		if len(volume) != 2 || volume[1] != ':' {
			return nil, fmt.Errorf("non-DOS reparse destination: %w", ErrOutside)
		}
		base := volume + `\`
		parent, err := openReparsePoint(base)
		if err != nil {
			return nil, err
		}
		parts := strings.FieldsFunc(strings.TrimPrefix(current[len(volume):], `\`), func(r rune) bool { return r == '\\' || r == '/' })
		if len(parts) == 0 {
			canonical, err := finalDOSPath(parent)
			if err != nil {
				_ = windows.CloseHandle(parent)
				return nil, err
			}
			return &Resolved{path: canonical, file: os.NewFile(uintptr(parent), canonical)}, nil
		}
		restarted := false
		for i, part := range parts {
			access := uint32(windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
			if i == len(parts)-1 {
				access = finalAccess
			}
			child, err := openRelative(parent, part, access)
			if err != nil {
				_ = windows.CloseHandle(parent)
				return nil, err
			}
			target, linked, err := reparseTarget(child)
			if err != nil {
				_ = windows.CloseHandle(child)
				_ = windows.CloseHandle(parent)
				return nil, err
			}
			if linked {
				hops++
				if hops > maxReparseHops {
					_ = windows.CloseHandle(child)
					_ = windows.CloseHandle(parent)
					return nil, fmt.Errorf("too many reparse points: %w", ErrInvalid)
				}
				next, err := localReparseDestination(filepath.Dir(filepath.Join(base, part)), target)
				_ = windows.CloseHandle(child)
				_ = windows.CloseHandle(parent)
				if err != nil {
					return nil, err
				}
				if i+1 < len(parts) {
					next = filepath.Join(append([]string{next}, parts[i+1:]...)...)
				}
				current, restarted = next, true
				break
			}
			_ = windows.CloseHandle(parent)
			if i == len(parts)-1 {
				canonical, err := finalDOSPath(child)
				if err != nil {
					_ = windows.CloseHandle(child)
					return nil, err
				}
				return &Resolved{path: canonical, file: os.NewFile(uintptr(child), canonical)}, nil
			}
			parent = child
			base = filepath.Join(base, part)
		}
		if restarted {
			continue
		}
	}
}

func openRelative(parent windows.Handle, name string, access uint32) (windows.Handle, error) {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	oa := &windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: parent, ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, access, oa, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return 0, err
	}
	return handle, nil
}

func openChild(parent *Resolved, name string, write, container bool) (*Resolved, error) {
	access := uint32(windows.FILE_GENERIC_READ)
	if container {
		access = windows.FILE_LIST_DIRECTORY | fileAddFile | fileAddSubdirectory | windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	}
	if write {
		access |= windows.DELETE | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE
	}
	h, err := openRelative(windows.Handle(parent.File().Fd()), name, access)
	if err != nil {
		return nil, err
	}
	_, linked, err := reparseTarget(h)
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	return &Resolved{path: filepath.Join(parent.Path(), name), file: os.NewFile(uintptr(h), name), reparse: linked}, nil
}

// resolve walks every component with OPEN_REPARSE_POINT. A junction is thus
// inspected before its target is opened, preventing SMB/NTLM authentication
// while an untrusted path is being authorized.
func resolve(path string) (string, error) {
	current := filepath.Clean(path)
	for hops := 0; ; {
		volume := filepath.VolumeName(current)
		if len(volume) != 2 || volume[1] != ':' {
			return "", fmt.Errorf("non-DOS reparse destination: %w", ErrOutside)
		}
		base := volume + `\`
		parts := strings.FieldsFunc(strings.TrimPrefix(current[len(volume):], `\`), func(r rune) bool { return r == '\\' || r == '/' })
		restarted := false
		for i, part := range parts {
			candidate := filepath.Join(base, part)
			h, err := openReparsePoint(candidate)
			if err != nil {
				return "", err
			}
			target, linked, err := reparseTarget(h)
			_ = windows.CloseHandle(h)
			if err != nil {
				return "", err
			}
			if !linked {
				base = candidate
				continue
			}
			hops++
			if hops > maxReparseHops {
				return "", fmt.Errorf("too many reparse points: %w", ErrInvalid)
			}
			next, err := localReparseDestination(filepath.Dir(candidate), target)
			if err != nil {
				return "", err
			}
			if i+1 < len(parts) {
				next = filepath.Join(append([]string{next}, parts[i+1:]...)...)
			}
			current, restarted = next, true
			break
		}
		if restarted {
			continue
		}
		h, err := openReparsePoint(base)
		if err != nil {
			return "", err
		}
		defer windows.CloseHandle(h)
		return finalDOSPath(h)
	}
}

func openReparsePoint(path string) (windows.Handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}

func reparseTarget(h windows.Handle) (string, bool, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return "", false, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return "", false, nil
	}
	buf := make([]byte, windows.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var n uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_GET_REPARSE_POINT, nil, 0, &buf[0], uint32(len(buf)), &n, nil); err != nil {
		return "", false, err
	}
	if n < 16 {
		return "", false, fmt.Errorf("short reparse buffer: %w", ErrInvalid)
	}
	tag := binary.LittleEndian.Uint32(buf[:4])
	if tag != windows.IO_REPARSE_TAG_MOUNT_POINT && tag != windows.IO_REPARSE_TAG_SYMLINK {
		// Unknown name-surrogate tags redirect name resolution. We cannot
		// inspect their target safely, so fail closed rather than traverse it.
		if tag&0x20000000 != 0 {
			return "", false, fmt.Errorf("unsupported name-surrogate reparse point: %w", ErrOutside)
		}
		return "", false, nil
	}
	off, length, start := int(binary.LittleEndian.Uint16(buf[8:10])), int(binary.LittleEndian.Uint16(buf[10:12])), 16
	if tag == windows.IO_REPARSE_TAG_SYMLINK {
		start = 20
	}
	if length%2 != 0 || start+off+length > int(n) {
		return "", false, fmt.Errorf("invalid reparse target: %w", ErrInvalid)
	}
	chars := make([]uint16, length/2)
	for i := range chars {
		chars[i] = binary.LittleEndian.Uint16(buf[start+off+i*2:])
	}
	return windows.UTF16ToString(chars), true, nil
}

// localReparseDestination rejects remote and device namespaces before they can
// be opened. Junction substitute names normally begin with \\??\\C:\\.
func localReparseDestination(parent, target string) (string, error) {
	t := strings.ReplaceAll(target, "/", `\`)
	lower := strings.ToLower(t)
	if strings.HasPrefix(lower, `\\??\unc\`) || strings.HasPrefix(lower, `\\?\unc\`) || strings.HasPrefix(lower, `\??\unc\`) || strings.HasPrefix(lower, `\?\unc\`) || strings.HasPrefix(lower, `\\`) || strings.HasPrefix(lower, `\device\`) || strings.HasPrefix(lower, `\??\volume{`) || strings.HasPrefix(lower, `\\?\volume{`) {
		return "", fmt.Errorf("remote or device reparse destination: %w", ErrOutside)
	}
	if strings.HasPrefix(lower, `\??\`) {
		t = t[4:]
	}
	if filepath.IsAbs(t) || (len(t) >= 3 && t[1] == ':' && t[2] == '\\') {
		if err := lexical(t); err != nil {
			return "", err
		}
		return filepath.Clean(t), nil
	}
	next := filepath.Clean(filepath.Join(parent, t))
	if err := lexical(next); err != nil {
		return "", err
	}
	return next, nil
}

func finalDOSPath(h windows.Handle) (string, error) {
	buf := make([]uint16, 32768)
	// Zero requests the documented FILE_NAME_NORMALIZED | VOLUME_NAME_DOS form.
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", fmt.Errorf("GetFinalPathNameByHandle: %w", err)
	}
	if n >= uint32(len(buf)) {
		return "", fmt.Errorf("GetFinalPathNameByHandle: ruta demasiado larga")
	}
	p := windows.UTF16ToString(buf[:n])
	if !strings.HasPrefix(strings.ToLower(p), `\\?\`) {
		return "", fmt.Errorf("non-DOS final path: %w", ErrOutside)
	}
	return strings.TrimPrefix(p, `\\?\`), nil
}
