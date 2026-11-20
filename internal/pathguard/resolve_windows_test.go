// SPDX-License-Identifier: Apache-2.0
//go:build windows

package pathguard

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
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

func TestResolveParentRaceCannotCreateInProtectedJunctionTarget(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "entry")
	canary := filepath.Join(root, "canary")
	if err := os.Mkdir(entry, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(canary, 0700); err != nil {
		t.Fatal(err)
	}
	g := Guard{Roots: []string{root}, Protected: []string{canary}}
	stop := make(chan struct{})
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		defer done.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(entry)
			makeJunctionBestEffort(entry, canary)
			_ = os.Remove(entry)
			_ = os.Mkdir(entry, 0700)
		}
	}()
	for i := 0; i < 100; i++ {
		r, err := g.ResolveParent(entry)
		if err != nil {
			continue // Junction observed or attacker changed it mid-resolution.
		}
		_ = createRelativeFile(r, "created-"+strconv.Itoa(i))
		_ = r.Close()
	}
	close(stop)
	done.Wait()
	entries, err := os.ReadDir(canary)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("race created entries in protected canary: %#v", entries)
	}
}

func makeJunctionBestEffort(link, target string) {
	_ = exec.Command("cmd", "/c", "mklink", "/J", link, target).Run()
}

func createRelativeFile(parent *Resolved, name string) error {
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return err
	}
	file := parent.File()
	h := windows.Handle(file.Fd())
	runtime.KeepAlive(file)
	oa := &windows.OBJECT_ATTRIBUTES{Length: uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})), RootDirectory: h, ObjectName: objectName, Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	var created windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&created, windows.FILE_GENERIC_WRITE, oa, &status, nil, windows.FILE_ATTRIBUTE_NORMAL, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err == nil {
		err = windows.CloseHandle(created)
	}
	return err
}
