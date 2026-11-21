// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package files

// Unix filesystems do not expose the Windows system-file attribute.
func isSystem(string) (bool, error) { return false, nil }
