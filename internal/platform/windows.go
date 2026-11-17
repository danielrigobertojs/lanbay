// SPDX-License-Identifier: Apache-2.0
//go:build windows

// Package platform contains the small operating-system boundary used by Lanbay.
package platform

import "golang.org/x/sys/windows"

// IsElevated reports whether the current token is elevated.
func IsElevated() bool {
	var token windows.Token
	return token.IsElevated()
}
