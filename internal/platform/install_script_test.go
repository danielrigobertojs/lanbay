// SPDX-License-Identifier: Apache-2.0
package platform

import (
	"strings"
	"testing"
)

func TestScheduledTaskScriptUsesEnvVar(t *testing.T) {
	script := scheduledTaskScript()
	if !strings.Contains(script, "$env:LANBAY_EXE") {
		t.Fatalf("script does not read LANBAY_EXE: %s", script)
	}
	if strings.Contains(script, `C:\`) {
		t.Fatalf("script embeds a path: %s", script)
	}
}

func TestInstallBinPath(t *testing.T) {
	if got := InstallBinPath(`C:\Program Files`); got != `C:\Program Files\Lanbay\lanbay.exe` {
		t.Fatalf("unexpected path: %s", got)
	}
}

func TestHasUntrustedWriter(t *testing.T) {
	if HasUntrustedWriter([]string{"S-1-5-32-544"}) {
		t.Fatal("administrators should be trusted")
	}
	if !HasUntrustedWriter([]string{"S-1-5-32-545"}) {
		t.Fatal("users should be untrusted")
	}
	if !HasUntrustedWriter([]string{"S-1-5-32-544", "S-1-5-32-545"}) {
		t.Fatal("mixed list should be untrusted")
	}
}

func TestHasWriteMaskTable(t *testing.T) {
	cases := []struct {
		mask uint32
		want bool
	}{
		{0x1200a9, false},
		{0x120089, false},
		{0x1301bf, true},
		{0x1f01ff, true},
		{0x40000, true},
		{0x80000, true},
		{0x10000, true},
		{0x40, true},
	}
	for _, c := range cases {
		if got := HasWriteMask(c.mask); got != c.want {
			t.Errorf("HasWriteMask(0x%x) = %v, want %v", c.mask, got, c.want)
		}
	}
}

func TestUntrustedWritersStandardProgramFiles(t *testing.T) {
	// Standard Program Files DACL: Users and ALL APPLICATION PACKAGES have RX
	// only; Administrators/SYSTEM have full control.
	entries := []ACLEntry{
		{Trustee: "S-1-5-32-544", Mask: 0x1f01ff, Allowed: true},
		{Trustee: "S-1-5-18", Mask: 0x1f01ff, Allowed: true},
		{Trustee: "S-1-5-32-545", Mask: 0x1200a9, Allowed: true},
		{Trustee: "S-1-15-2-1", Mask: 0x1200a9, Allowed: true, InheritOnly: false},
	}
	if got := UntrustedWritersFromEntries(entries); len(got) != 0 {
		t.Fatalf("expected no untrusted writers, got %v", got)
	}
}

func TestTaskkillOtherInstancesArgs(t *testing.T) {
	args := TaskkillOtherInstancesArgs(1234)
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	if !containsAll(joined, "taskkill.exe", "lanbay.exe", "PID ne 1234") {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestStopRunningInstanceCommands(t *testing.T) {
	cmds := StopRunningInstanceCommands(42)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestIsUnderInstallDir(t *testing.T) {
	pf := `C:\Program Files`
	cases := []struct {
		path string
		want bool
	}{
		{`C:\Program Files\Lanbay\lanbay.exe`, true},
		{`c:\program files\lanbay\lanbay.exe`, true},
		{`C:\Program Files\LanbayX\lanbay.exe`, false},
		{`C:\Users\foo\lanbay.exe`, false},
		{`D:\Program Files\Lanbay\lanbay.exe`, false},
	}
	for _, c := range cases {
		if got := IsUnderInstallDir(c.path, pf); got != c.want {
			t.Errorf("IsUnderInstallDir(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
