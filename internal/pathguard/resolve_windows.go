// SPDX-License-Identifier: Apache-2.0
//go:build windows

package pathguard

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// resolve asks the kernel for the final pathname of an opened handle. This is
// deliberately not filepath.EvalSymlinks: junctions and symlinks are resolved
// by Windows before authorization.
func resolve(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", fmt.Errorf("GetFinalPathNameByHandle: %w", err)
	}
	if n >= uint32(len(buf)) {
		return "", fmt.Errorf("GetFinalPathNameByHandle: ruta demasiado larga")
	}
	return strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`), nil
}
