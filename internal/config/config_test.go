// SPDX-License-Identifier: Apache-2.0
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadCreatesSecureDefault(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if c.Listen != ":8443" || len(c.AllowedNetworks) == 0 {
		t.Fatalf("unexpected defaults: %#v", c)
	}
	if _, err := Load(filepath.Dir(c.DataDir)); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultConfigurationPersistsOnlyNamedHosts(t *testing.T) {
	dir := t.TempDir()
	c := Default(dir)
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatal(err)
	}
	for _, ip := range interfaceIPs() {
		if strings.Contains(string(b), `"`+ip+`"`) {
			t.Fatalf("runtime address %q was persisted", ip)
		}
	}
}

func TestProtectedPathsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Default(dir)
	c.ProtectedPaths = []string{filepath.Join(dir, "keep")}
	if err := os.Mkdir(c.ProtectedPaths[0], 0700); err != nil {
		t.Fatal(err)
	}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ProtectedPaths) != 1 || got.ProtectedPaths[0] != c.ProtectedPaths[0] {
		t.Fatalf("protected paths = %#v", got.ProtectedPaths)
	}
}

func TestRuntimeHostsReturnsCopyOfCache(t *testing.T) {
	hostCache.Lock()
	hostCache.hosts = []string{"192.168.1.7"}
	hostCache.expires = time.Now().Add(time.Hour)
	hostCache.Unlock()
	first := RuntimeHosts()
	first[0] = "mutated"
	if got := RuntimeHosts()[0]; got != "192.168.1.7" {
		t.Fatalf("cache was mutated through result: %q", got)
	}
}
