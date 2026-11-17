// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package platform

// IsElevated is not meaningful on development platforms yet.
func IsElevated() bool { return false }
