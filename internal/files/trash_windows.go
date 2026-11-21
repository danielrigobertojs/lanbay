// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	clsctxInprocServer  = 0x1
	fofSilent           = 0x0004
	fofNoConfirmation   = 0x0010
	fofNoErrorUI        = 0x0400
	fofxRecycleOnDelete = 0x00080000
)

var (
	ole32                           = windows.NewLazySystemDLL("ole32.dll")
	shell32                         = windows.NewLazySystemDLL("shell32.dll")
	procCoCreateInstance            = ole32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")
	clsidFileOperation              = mustGUID("{3AD05575-8857-4850-9277-11B85BDB8E09}")
	iidFileOperation                = mustGUID("{947AAB5F-0A5C-4C13-B4D6-4BF7836A9E2}")
	iidShellItem                    = mustGUID("{43826D1E-E718-42EE-BC55-A1E261C37BFE}")
)

type windowsTrash struct{}
type comObject struct{ vtbl *[23]uintptr }

func NewTrash() Trash { return windowsTrash{} }

func (windowsTrash) Delete(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		return fmt.Errorf("initialize recycle COM apartment: %w", err)
	}
	defer windows.CoUninitialize()

	var operation *comObject
	r, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, clsctxInprocServer, uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&operation)))
	if err := hresult(r); err != nil {
		return fmt.Errorf("create IFileOperation: %w", err)
	}
	defer operation.release()
	item, err := shellItem(path)
	if err != nil {
		return err
	}
	defer item.release()
	if err := operation.call(5, uintptr(fofSilent|fofNoConfirmation|fofNoErrorUI|fofxRecycleOnDelete)); err != nil {
		return fmt.Errorf("set recycle flags: %w", err)
	}
	if err := operation.call(18, uintptr(unsafe.Pointer(item)), 0); err != nil {
		return fmt.Errorf("schedule recycle delete: %w", err)
	}
	if err := operation.call(21); err != nil {
		return fmt.Errorf("perform recycle delete: %w", err)
	}
	var aborted int32
	if err := operation.call(22, uintptr(unsafe.Pointer(&aborted))); err != nil {
		return fmt.Errorf("check recycle abort: %w", err)
	}
	if aborted != 0 {
		return fmt.Errorf("recycle delete aborted")
	}
	return nil
}

func shellItem(path string) (*comObject, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var item *comObject
	r, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if err := hresult(r); err != nil {
		return nil, fmt.Errorf("create shell item: %w", err)
	}
	return item, nil
}
func (o *comObject) call(index int, args ...uintptr) error {
	call := append([]uintptr{o.vtbl[index], uintptr(unsafe.Pointer(o))}, args...)
	r, _, _ := syscall.SyscallN(call[0], call[1:]...)
	return hresult(r)
}
func (o *comObject) release() { _, _, _ = syscall.SyscallN(o.vtbl[2], uintptr(unsafe.Pointer(o))) }
func hresult(r uintptr) error {
	if int32(r) < 0 {
		return syscall.Errno(uint32(r))
	}
	return nil
}
func mustGUID(s string) windows.GUID {
	g, err := windows.GUIDFromString(s)
	if err != nil {
		panic(err)
	}
	return g
}
