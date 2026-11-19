// SPDX-License-Identifier: Apache-2.0
package server

import (
	"bytes"
	"context"
	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/store"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
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

func testServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := config.Default(dir)
	c.AllowedHosts = []string{"localhost", "::1"}
	s := New(c, db, nil, "test")
	if err := s.auth.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	return s
}
func request(s *Server, method, target, host, origin string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	r.Host = host
	r.RemoteAddr = "127.0.0.1:1234"
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestHandlerRejectsHostOriginAndLargeBody(t *testing.T) {
	s := testServer(t)
	if got := request(s, http.MethodPost, "https://bad/api/v1/auth/login", "bad:8443", "", nil).Code; got != http.StatusMisdirectedRequest {
		t.Fatalf("host=%d", got)
	}
	if got := request(s, http.MethodPost, "https://localhost/api/v1/auth/login", "localhost:8443", "https://evil.example", nil).Code; got != http.StatusForbidden {
		t.Fatalf("origin=%d", got)
	}
	if got := request(s, http.MethodPost, "https://localhost/api/v1/auth/login", "localhost", "", []byte(`{"password":"`+strings.Repeat("a", 1<<20)+`"}`)).Code; got != http.StatusRequestEntityTooLarge {
		t.Fatalf("body=%d", got)
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
