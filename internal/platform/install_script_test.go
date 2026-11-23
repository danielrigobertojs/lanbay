// SPDX-License-Identifier: Apache-2.0
package platform

import (
	"path/filepath"
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
	if got := InstallBinPath(`C:\Program Files`); got != filepath.Join(`C:\Program Files`, "Lanbay", "lanbay.exe") {
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
