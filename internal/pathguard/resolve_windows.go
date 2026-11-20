// SPDX-License-Identifier: Apache-2.0
//go:build windows

package pathguard

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const maxReparseHops = 8

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
	if strings.HasPrefix(lower, `\\??\unc\`) || strings.HasPrefix(lower, `\\?\unc\`) || strings.HasPrefix(lower, `\\`) || strings.HasPrefix(lower, `\device\`) || strings.HasPrefix(lower, `\??\volume{`) || strings.HasPrefix(lower, `\\?\volume{`) {
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
