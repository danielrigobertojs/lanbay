// SPDX-License-Identifier: Apache-2.0
package software

import "testing"

func TestDiffUninstallKeysNone(t *testing.T) {
	before := RegistrySnapshot{"a": {Key: "a", DisplayName: "A"}}
	after := RegistrySnapshot{"a": {Key: "a", DisplayName: "A"}}
	if got := DiffUninstallKeys(before, after); len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestDiffUninstallKeysOne(t *testing.T) {
	before := RegistrySnapshot{}
	after := RegistrySnapshot{"new": {Key: "new", DisplayName: "New", DisplayVersion: "1.0"}}
	got := DiffUninstallKeys(before, after)
	if len(got) != 1 || got[0].DisplayName != "New" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestDiffUninstallKeysMultiple(t *testing.T) {
	before := RegistrySnapshot{}
	after := RegistrySnapshot{
		"a": {Key: "a", DisplayName: "A"},
		"b": {Key: "b", DisplayName: "B"},
	}
	got := DiffUninstallKeys(before, after)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestDiffUninstallKeysIgnoresPreexisting(t *testing.T) {
	before := RegistrySnapshot{"old": {Key: "old", DisplayName: "Old"}}
	after := RegistrySnapshot{"old": {Key: "old", DisplayName: "Old"}, "new": {Key: "new", DisplayName: "New"}}
	got := DiffUninstallKeys(before, after)
	if len(got) != 1 || got[0].Key != "new" {
		t.Fatalf("unexpected: %+v", got)
	}
}
