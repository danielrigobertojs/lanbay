// SPDX-License-Identifier: Apache-2.0
package software

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStagingPath(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	// legitimate file directly in staging
	good := filepath.Join(staging, "app.msi")
	if err := os.WriteFile(good, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"bare name ok", "app.msi", false},
		{"absolute inside ok", good, false},
		{"outside absolute", filepath.Join(root, "secret.txt"), true},
		{"traversal", "a\\..\\..\\x.exe", true},
		{"parent staging dir", staging, true},
		{"wrong extension", "note.txt", true},
		{"ads", "app.msi:stream", true},
		{"unc", `\\server\share\app.msi`, true},
		{"device prefix", `\\?\C:\x\app.msi`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveStagingPath(staging, tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestResolveStagingPathRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	if err := os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.exe")
	if err := os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(staging, "link.exe")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("symlink not supported")
	}
	if _, err := ResolveStagingPath(staging, "link.exe"); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
