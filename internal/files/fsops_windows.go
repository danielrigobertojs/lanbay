// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"unsafe"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
	"golang.org/x/sys/windows"
)

type fileRenameInformationEx struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [1]uint16
}

func mkdirAt(parent *pathguard.Resolved, name string, _ os.FileMode) error {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return err
	}
	h, err := fileHandle(parent)
	if err != nil {
		return err
	}
	oa := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: h,
		ObjectName:    objectName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	var created windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&created, windows.FILE_GENERIC_READ|windows.FILE_GENERIC_WRITE, oa, &status, nil, windows.FILE_ATTRIBUTE_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_CREATE,
		windows.FILE_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	return windows.CloseHandle(created)
}

func renameAt(source, parent *pathguard.Resolved, name string) error {
	sourceHandle, err := fileHandle(source)
	if err != nil {
		return err
	}
	parentHandle, err := fileHandle(parent)
	if err != nil {
		return err
	}
	name16, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	nameBytes := (len(name16) - 1) * 2
	var header fileRenameInformationEx
	size := int(unsafe.Offsetof(header.FileName)) + nameBytes
	buf := make([]byte, size)
	info := (*fileRenameInformationEx)(unsafe.Pointer(&buf[0]))
	info.RootDirectory = parentHandle
	info.FileNameLength = uint32(nameBytes)
	copy((*[windows.MAX_LONG_PATH]uint16)(unsafe.Pointer(&info.FileName[0]))[:len(name16)-1], name16[:len(name16)-1])
	err = windows.SetFileInformationByHandle(sourceHandle, windows.FileRenameInfoEx, &buf[0], uint32(len(buf)))
	if exUnsupported(err) {
		// FAT-family filesystems do not support FileRenameInfoEx. The legacy
		// information class retains the same RootDirectory handle, so it does
		// not reopen an untrusted destination path.
		err = windows.SetFileInformationByHandle(sourceHandle, windows.FileRenameInfo, &buf[0], uint32(len(buf)))
	}
	if err != nil {
		return fmt.Errorf("rename by handle: %w", err)
	}
	return nil
}

func deleteAt(resolved *pathguard.Resolved) error {
	info, err := resolved.File().Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return ErrRecursiveDelete
	}
	h, err := fileHandle(resolved)
	if err != nil {
		return err
	}
	flags := uint32(windows.FILE_DISPOSITION_DELETE | windows.FILE_DISPOSITION_POSIX_SEMANTICS | windows.FILE_DISPOSITION_IGNORE_READONLY_ATTRIBUTE)
	err = windows.SetFileInformationByHandle(h, windows.FileDispositionInfoEx, (*byte)(unsafe.Pointer(&flags)), uint32(unsafe.Sizeof(flags)))
	if exUnsupported(err) {
		// FAT32 lacks the extended disposition class. Retry against this exact
		// validated handle using the legacy, portable form.
		legacy := uint32(1)
		err = windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, (*byte)(unsafe.Pointer(&legacy)), uint32(unsafe.Sizeof(legacy)))
	}
	if err != nil {
		return fmt.Errorf("delete by handle: %w", err)
	}
	return nil
}

func exUnsupported(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED)
}

func copyFileAt(source, parent *pathguard.Resolved, name string, _ os.FileMode) error {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return err
	}
	parentHandle, err := fileHandle(parent)
	if err != nil {
		return err
	}
	oa := &windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: parentHandle, ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	var destination windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&destination, windows.FILE_GENERIC_WRITE, oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_CREATE,
		windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return err
	}
	out := os.NewFile(uintptr(destination), name)
	_, copyErr := io.Copy(out, source.File())
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileHandle(resolved *pathguard.Resolved) (windows.Handle, error) {
	if resolved == nil || resolved.File() == nil {
		return 0, fmt.Errorf("closed resolved handle")
	}
	file := resolved.File()
	h := windows.Handle(file.Fd())
	runtime.KeepAlive(file)
	return h, nil
}
