// SPDX-License-Identifier: Apache-2.0
package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectsHostilePaths(t *testing.T) {
	for _, path := range []string{`\\server\share\x`, `\\?\UNC\server\share\x`, `//server/share/x`, `\\.\PhysicalDrive0`, `\\?\GLOBALROOT\Device`, `C:\a\..\b`, `C:\x::$DATA`, `C:\CON`, `C:\COM1.txt`, `C:\name.`, `C:\name `, `relative\x`} {
		if lexical(path) == nil {
			t.Errorf("accepted %q", path)
		}
	}
}
func TestProtectedAndRoot(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "Windows")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	g := Guard{Roots: []string{root}, Protected: []string{protected}}
	if _, err := g.Check(root, true); !errors.Is(err, ErrRoot) {
		t.Fatalf("root: %v", err)
	}
	if _, err := g.Check(protected, true); !errors.Is(err, ErrProtected) {
		t.Fatalf("protected: %v", err)
	}
	if _, err := g.Check(protected, false); err != nil {
		t.Fatalf("read: %v", err)
	}
}
