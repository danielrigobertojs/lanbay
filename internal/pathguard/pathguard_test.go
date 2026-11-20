// SPDX-License-Identifier: Apache-2.0
package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRejectsHostilePaths(t *testing.T) {
	for _, path := range []string{`\\server\share\x`, `\\?\UNC\server\share\x`, `//server/share/x`, `\\.\PhysicalDrive0`, `\\?\GLOBALROOT\Device`, `\??\C:\x`, `\\?\Volume{abc}\x`, `C:\a\..\b`, `C:/a\\..\\b`, "C:\\x\x00y", `C:\x::$DATA`, `C:\x::$INDEX_ALLOCATION`, `C:\x:stream`, `C:\CON`, `C:\CON .txt`, `C:\COM0.txt`, `C:\LPT¹`, `C:\name.`, `C:\name `, `C:\a<b`, `relative\x`} {
		if lexical(path) == nil {
			t.Errorf("accepted %q", path)
		}
	}
}

func TestComponentBoundariesAndAncestorProtection(t *testing.T) {
	if within(`C:\Program Files (x86)`, `C:\Program Files`) {
		t.Fatal("prefix is not a component")
	}
	if !within(`C:\Users\x`, `C:\`) {
		t.Fatal("volume root must contain children")
	}
	root := t.TempDir()
	protected := filepath.Join(root, "real", "protected")
	if err := os.MkdirAll(protected, 0700); err != nil {
		t.Fatal(err)
	}
	g := Guard{Roots: []string{root}, Protected: []string{protected}}
	if _, err := g.Check(filepath.Join(root, "real"), true); !errors.Is(err, ErrProtected) {
		t.Fatalf("ancestor: %v", err)
	}
	newPath, err := g.CheckNew(root, "new.txt", true)
	if err != nil || filepath.Base(newPath) != "new.txt" {
		t.Fatalf("CheckNew: %q, %v", newPath, err)
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

func TestCheckTransferRejectsProtectedSourceAndDestination(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "Windows")
	user := filepath.Join(root, "Users", "owner.txt")
	if err := os.MkdirAll(filepath.Dir(user), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(user, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	g := Guard{Roots: []string{root}, Protected: []string{protected}}
	if _, _, err := g.CheckTransfer(protected, user); !errors.Is(err, ErrProtected) {
		t.Fatalf("protected source: %v", err)
	}
	if _, _, err := g.CheckTransfer(user, protected); !errors.Is(err, ErrProtected) {
		t.Fatalf("protected destination: %v", err)
	}
}

func TestCheckNewRejectsSeparatorsAndEscapes(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "safe")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	g := Guard{Roots: []string{root}}
	for _, name := range []string{`..\\Windows\\evil`, "../Windows/evil", `a\\..\\..\\Windows\\evil`, filepath.Join(root, "absolute")} {
		if _, err := g.CheckNew(parent, name, true); !errors.Is(err, ErrInvalid) {
			t.Errorf("name %q: %v", name, err)
		}
	}
}
