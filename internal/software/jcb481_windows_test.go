// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJCB481RetainedHandleBlocksWrite(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openRetained(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
		t.Fatal("expected sharing violation")
	}
}
