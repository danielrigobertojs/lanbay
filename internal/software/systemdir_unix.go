// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import "os"

func systemDirectory() string {
	if v := os.Getenv("SystemRoot"); v != "" {
		return v + "/System32"
	}
	return "/usr/bin"
}
