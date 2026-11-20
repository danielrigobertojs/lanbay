// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import "testing"

func TestNewTrashLoads(t *testing.T) {
	tr := NewTrash()
	if tr == nil {
		t.Fatal("NewTrash returned nil")
	}
}
