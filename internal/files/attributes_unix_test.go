// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package files

import "testing"

func TestIsSystemIsFalseOffWindows(t *testing.T) {
	system, err := isSystem(t.TempDir())
	if err != nil || system {
		t.Fatalf("isSystem = %t, %v", system, err)
	}
}
