// SPDX-License-Identifier: Apache-2.0
//go:build windows

// Package platform contains the small operating-system boundary used by Lanbay.
package platform

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const processRedirectionTrustPolicy = 16

var (
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	setProcessMitigationPolicy = kernel32.NewProc("SetProcessMitigationPolicy")
)

// IsElevated reports whether the current token is elevated.
func IsElevated() bool {
	var token windows.Token
	return token.IsElevated()
}

// EnforceRedirectionTrust asks Windows 11 22H2+ not to follow redirections
// created by a less-trusted token. It is defence in depth; callers continue
// when it is unavailable and pathguard remains the primary boundary.
func EnforceRedirectionTrust() error {
	if err := setProcessMitigationPolicy.Find(); err != nil {
		return fmt.Errorf("find SetProcessMitigationPolicy: %w", err)
	}
	policy := uint32(1) // PROCESS_MITIGATION_REDIRECTION_TRUST_POLICY.EnforceRedirectionTrust
	r1, _, callErr := syscall.SyscallN(
		setProcessMitigationPolicy.Addr(),
		uintptr(processRedirectionTrustPolicy),
		uintptr(unsafe.Pointer(&policy)),
		unsafe.Sizeof(policy),
	)
	if r1 == 0 {
		return fmt.Errorf("set ProcessRedirectionTrustPolicy: %w", callErr)
	}
	return nil
}
