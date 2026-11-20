// SPDX-License-Identifier: Apache-2.0
//go:build windows

package pathguard

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalReparseDestinationRejectsRemoteBeforeOpen(t *testing.T) {
	for _, target := range []string{
		`\\127.0.0.1\c$`,
		`\??\UNC\127.0.0.1\c$`,
		`\?\UNC\127.0.0.1\c$`,
		`\\??\UNC\127.0.0.1\c$`,
		`\\?\UNC\attacker\share`,
		`\Device\Mup\attacker\share`,
		`\??\Volume{01234567-89ab-cdef-0123-456789abcdef}\`,
	} {
		if _, err := localReparseDestination(`C:\safe`, target); !errors.Is(err, ErrOutside) {
			t.Errorf("target %q error = %v, want outside_allowed_volume", target, err)
		}
	}
}

func TestLocalReparseDestinationAcceptsLocalTargets(t *testing.T) {
	for _, target := range []string{`C:\safe\child`, `\??\C:\safe\child`, `child`} {
		got, err := localReparseDestination(`C:\safe`, target)
		if err != nil {
			t.Fatalf("target %q: %v", target, err)
		}
		if got == "" {
			t.Fatalf("target %q returned empty path", target)
		}
	}
}

func TestResolveFollowsLocalJunctionWithinAllowedVolume(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	link := filepath.Join(root, "link")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(target, "inside.txt")
	if err := os.WriteFile(file, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	makeJunction(t, link, target)
	defer os.Remove(link)

	r, err := (Guard{Roots: []string{root}}).Resolve(filepath.Join(link, "inside.txt"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	want, err := resolve(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(r.Path(), want) {
		t.Fatalf("resolved path = %q, want %q", r.Path(), want)
	}
}

func makeJunction(t *testing.T, link, target string) {
	t.Helper()
	output, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create junction: %v: %s", err, strings.TrimSpace(string(output)))
	}
}
