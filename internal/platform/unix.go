// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package platform

// IsElevated is not meaningful on development platforms yet.
func IsElevated() bool { return false }

// EnforceRedirectionTrust is only available on Windows.
func EnforceRedirectionTrust() error { return nil }
