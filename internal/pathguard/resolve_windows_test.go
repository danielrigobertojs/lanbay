// SPDX-License-Identifier: Apache-2.0
//go:build windows

package pathguard

import (
	"errors"
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
