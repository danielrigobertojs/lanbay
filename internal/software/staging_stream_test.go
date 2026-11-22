// SPDX-License-Identifier: Apache-2.0
package software

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageInstallerStreamMagicInvalid(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("A"), 16)
	if _, err := StageInstallerStream(staging, "bad.exe", bytes.NewReader(data)); err == nil {
		t.Fatal("expected magic error")
	}
	// no residual .part
	entries, _ := os.ReadDir(staging)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Fatalf("residual part file: %s", e.Name())
		}
	}
}

func TestStageInstallerStreamSuccess(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	data := append([]byte{0x4D, 0x5A}, bytes.Repeat([]byte("A"), 16)...)
	name, err := StageInstallerStream(staging, "ok.exe", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".exe") {
		t.Fatalf("unexpected name: %s", name)
	}
	if _, err := os.Stat(filepath.Join(staging, name)); err != nil {
		t.Fatal(err)
	}
}
