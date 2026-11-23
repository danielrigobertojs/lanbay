// SPDX-License-Identifier: Apache-2.0
package platform

import (
	"strings"
	"testing"
)

func TestScheduledTaskScriptEscapesApostrophe(t *testing.T) {
	path := `C:\Program Files\O'Brien\lanbay.exe`
	script := scheduledTaskScript(path)
	if strings.Contains(script, path) {
		t.Fatalf("script contains unescaped path: %s", script)
	}
	escaped := `C:\Program Files\O''Brien\lanbay.exe`
	if !strings.Contains(script, escaped) {
		t.Fatalf("script does not contain escaped path %q: %s", escaped, script)
	}
}

func TestScheduledTaskScriptPlainPath(t *testing.T) {
	path := `C:\Lanbay\lanbay.exe`
	script := scheduledTaskScript(path)
	if !strings.Contains(script, path) {
		t.Fatalf("script does not contain path: %s", script)
	}
}
