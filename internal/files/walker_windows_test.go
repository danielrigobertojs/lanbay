// SPDX-License-Identifier: Apache-2.0
//go:build windows

package files

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

func TestDirectoryWalkerSkipsLocalAndRemoteReparsePoints(t *testing.T) {
	root := t.TempDir()
	system32 := filepath.Join(os.Getenv("SystemRoot"), "System32")
	if _, err := os.Stat(system32); err != nil {
		t.Fatalf("System32 unavailable: %v", err)
	}
	for _, target := range []string{system32, `\\127.0.0.1\c$`} {
		t.Run(target, func(t *testing.T) {
			source := filepath.Join(root, strings.NewReplacer(`\`, "_", ":", "_").Replace(target))
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(source, "link")
			makeDirectoryLink(t, link, target)
			m := Manager{Guard: pathguard.Guard{Roots: []string{root}}}
			if err := m.Copy(context.Background(), source, root, filepath.Base(source)+"-copy"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(filepath.Join(root, filepath.Base(source)+"-copy", "link")); !os.IsNotExist(err) {
				t.Fatalf("reparse point copied: %v", err)
			}
			var data bytes.Buffer
			if err := m.Zip(context.Background(), source, &data); err != nil {
				t.Fatal(err)
			}
			zr, err := zip.NewReader(bytes.NewReader(data.Bytes()), int64(data.Len()))
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range zr.File {
				if strings.Contains(strings.ToLower(f.Name), "link") {
					t.Fatalf("reparse point zipped: %q", f.Name)
				}
			}
			if err := m.Delete(context.Background(), source, DeleteOptions{Permanent: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(system32); err != nil {
				t.Fatalf("System32 changed: %v", err)
			}
		})
	}
}

func TestDirectoryWalkerRaceDoesNotWriteThroughJunction(t *testing.T) {
	root := t.TempDir()
	source, canary := filepath.Join(root, "source"), filepath.Join(root, "canary")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(canary, 0700); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(source, "entry")
	if err := os.Mkdir(entry, 0700); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(entry)
			_ = exec.Command("cmd", "/c", "mklink", "/J", entry, canary).Run()
			_ = os.Remove(entry)
			_ = os.Mkdir(entry, 0700)
		}
	}()
	m := Manager{Guard: pathguard.Guard{Roots: []string{root}, Protected: []string{canary}}}
	for i := 0; i < 100; i++ {
		_ = m.Copy(context.Background(), source, root, "copy-"+strconv.Itoa(i))
	}
	close(stop)
	wg.Wait()
	entries, err := os.ReadDir(canary)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("race wrote through junction: %#v", entries)
	}
}

func TestWalkLimitsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newWalkCount().add(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled walk: %v", err)
	}
	if err := newWalkCount().add(context.Background(), maxWalkDepth+1); err == nil {
		t.Fatal("depth limit accepted")
	}
	if err := (&walkCount{entries: maxWalkEntries}).add(context.Background(), 0); err == nil {
		t.Fatal("entry limit accepted")
	}
}

func makeDirectoryLink(t *testing.T, link, target string) {
	t.Helper()
	args := []string{"/c", "mklink", "/D", link, target}
	if !strings.HasPrefix(target, `\\`) {
		args = []string{"/c", "mklink", "/J", link, target}
	}
	out, err := exec.Command("cmd", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink %q: %v: %s", target, err, strings.TrimSpace(string(out)))
	}
}
