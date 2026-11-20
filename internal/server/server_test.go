// SPDX-License-Identifier: Apache-2.0
package server

import (
	"bytes"
	"context"
	"errors"
	"github.com/danielrigobertojs/lanbay/internal/auth"
	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/files"
	"github.com/danielrigobertojs/lanbay/internal/pathguard"
	"github.com/danielrigobertojs/lanbay/internal/store"
	"github.com/danielrigobertojs/lanbay/internal/volumes"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fixedVolumes []volumes.Volume

func (v fixedVolumes) List(context.Context) ([]volumes.Volume, error) { return v, nil }

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

func TestRecycleAvailable(t *testing.T) {
	items := []volumes.Volume{{ID: `C:\\`, Recycle: true}, {ID: `D:\\`, Recycle: false}}
	if !recycleAvailable(`c:\\Users\\owner\\file.txt`, items) {
		t.Fatal("fixed NTFS volume should recycle")
	}
	if recycleAvailable(`D:\\file.txt`, items) {
		t.Fatal("removable volume must require permanent confirmation")
	}
	if recycleAvailable(`E:\\file.txt`, items) {
		t.Fatal("unknown volume must fail closed")
	}
}

func TestFileErrorExplainsTemporarilyUnsupportedFolderOperations(t *testing.T) {
	for _, err := range []error{files.ErrRecursiveDelete, files.ErrDirectoryCopy, files.ErrDirectoryZip} {
		w := httptest.NewRecorder()
		fileError(w, err)
		if got := strings.TrimSpace(w.Body.String()); got != "Aún no disponible para carpetas en este volumen (JCB-473)" {
			t.Errorf("fileError(%v) = %q", err, got)
		}
	}
}

func TestFileManagerProtectsConfiguredZones(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	protected := filepath.Join(root, "protected")
	for _, path := range []string{data, protected} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	c := config.Default(data)
	c.ProtectedPaths = []string{protected}
	s := New(c, nil, fixedVolumes{{ID: root}}, "test")
	m, err := s.fileManager(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Guard.Check(protected, true); !errors.Is(err, pathguard.ErrProtected) {
		t.Fatalf("write to configured protected zone = %v", err)
	}
	if _, err := m.Guard.Check(root, true); !errors.Is(err, pathguard.ErrRoot) {
		t.Fatalf("write to volume root = %v", err)
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

func TestPasswordEndpointRequiresCurrentPassword(t *testing.T) {
	s := testServer(t)
	id, csrf, _, err := s.auth.Login(context.Background(), netip.MustParseAddr("127.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"password":"new correct battery"}`, `{"current_password":"wrong password value","password":"new correct battery"}`} {
		r := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/auth/password", strings.NewReader(body))
		r.Host = "localhost"
		r.RemoteAddr = "127.0.0.1:1234"
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: id})
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("body %q: %d", body, w.Code)
		}
	}
}

func TestPasswordEndpointAuditsAndRevokesAfterFiveFailures(t *testing.T) {
	s := testServer(t)
	id, csrf, _, err := s.auth.Login(context.Background(), netip.MustParseAddr("127.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		r := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/auth/password", strings.NewReader(`{"current_password":"wrong password value","password":"new correct battery"}`))
		r.Host = "localhost"
		r.RemoteAddr = "127.0.0.1:1234"
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: id})
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: %d", i+1, w.Code)
		}
	}
	if ok, _ := s.auth.Check(context.Background(), id, csrf, false); ok {
		t.Fatal("session remains valid after five password-change failures")
	}
	var audits int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='password_change' AND result='failed'").Scan(&audits); err != nil || audits != 5 {
		t.Fatalf("failed password-change audits = %d, %v", audits, err)
	}
}

func TestPasswordEndpointConcurrentFailuresAreBounded(t *testing.T) {
	s := testServer(t)
	id, csrf, _, err := s.auth.Login(context.Background(), netip.MustParseAddr("127.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	const requests = 20
	start := make(chan struct{})
	codes := make(chan int, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/auth/password", strings.NewReader(`{"current_password":"wrong password value","password":"new correct battery"}`))
			r.Host = "localhost"
			r.RemoteAddr = "127.0.0.1:1234"
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: id})
			r.Header.Set("X-CSRF-Token", csrf)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			codes <- w.Code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	for code := range codes {
		if code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", code)
		}
	}
	var audits int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='password_change' AND result='failed'").Scan(&audits); err != nil || audits != 5 {
		t.Fatalf("failed password-change audits = %d, %v", audits, err)
	}
	if ok, _ := s.auth.Check(context.Background(), id, csrf, false); ok {
		t.Fatal("session remains valid after concurrent failures")
	}
	r := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/auth/password", strings.NewReader(`{"current_password":"correct horse battery","password":"new correct battery"}`))
	r.Host = "localhost"
	r.RemoteAddr = "127.0.0.1:1234"
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: id})
	r.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("correct password after revocation status = %d, want 401", w.Code)
	}
	if !s.auth.VerifyPassword(context.Background(), "correct horse battery") || s.auth.VerifyPassword(context.Background(), "new correct battery") {
		t.Fatal("password changed after session revocation")
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
