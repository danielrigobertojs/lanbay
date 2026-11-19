// SPDX-License-Identifier: Apache-2.0
package server

import (
	"github.com/danielrigobertojs/lanbay/internal/config"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestAllowedIP(t *testing.T) {
	networks := config.DefaultNetworks()
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.0.1", "fc00::1"} {
		if !AllowedIP(netip.MustParseAddr(s), networks) {
			t.Errorf("%s should be allowed", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "172.15.1.1", "100.64.1.1", "fe00::1"} {
		if AllowedIP(netip.MustParseAddr(s), networks) {
			t.Errorf("%s should be rejected", s)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest(http.MethodGet, "https://localhost/", nil)
	w := httptest.NewRecorder()
	s.headers(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(w, r)
	if got := w.Header().Get("Content-Security-Policy"); got == "" || !containsAll(got, "script-src", "style-src", "img-src", "connect-src", "object-src") {
		t.Fatalf("incomplete CSP: %q", got)
	}
	if w.Header().Get("Cross-Origin-Opener-Policy") != "same-origin" || w.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatal("cross-origin headers missing")
	}
}
func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func TestAllowedHost(t *testing.T) {
	allowed := []string{"panel.local", "192.168.1.2", "::1"}
	for _, host := range []string{"panel.local", "PANEL.LOCAL", "[::1]"} {
		if !allowedHost(host, allowed) {
			t.Errorf("host %q rejected", host)
		}
	}
	if allowedHost("attacker.example", allowed) {
		t.Fatal("unexpected host accepted")
	}
}
