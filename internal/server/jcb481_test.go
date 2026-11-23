// SPDX-License-Identifier: Apache-2.0
package server

import (
	"bytes"
	"context"
	"github.com/danielrigobertojs/lanbay/internal/auth"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/jobs"
	"github.com/danielrigobertojs/lanbay/internal/software"
	"github.com/danielrigobertojs/lanbay/internal/store"
)

func jcb481ServerWithSoftware(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	c := config.Default(dir)
	s := New(c, db, nil, "test")
	if err := s.auth.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	jm, err := jobs.New(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := software.NewService(db, software.NewDisabledBackend("disabled"), jm, dir)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSoftware(svc)
	return s
}

func jcb481MultipartBody(t *testing.T, fields map[string]string, fileContent []byte, fileName string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if fileContent != nil {
		fw, err := mw.CreateFormFile("file", fileName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write(fileContent); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, mw.FormDataContentType()
}

func TestJCB481StagingNoReauthReturns401(t *testing.T) {
	s := jcb481ServerWithSoftware(t)
	id, csrf, _, err := s.auth.Login(context.Background(), netip.MustParseAddr("127.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	content := append([]byte{0x4D, 0x5A}, bytes.Repeat([]byte("A"), 8)...)
	body, ct := jcb481MultipartBody(t, nil, content, "ok.exe")
	req := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/software/staging", body)
	req.Host = "localhost"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", ct)
	req.AddCookie(&http.Cookie{Name: authCookieName(), Value: id})
	req.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}
	// no .part files
	entries, _ := os.ReadDir(filepath.Join(s.config.DataDir, "staging"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Fatalf("residual part file: %s", e.Name())
		}
	}
}

func TestJCB481StagingReauthAfterFileReturns401(t *testing.T) {
	s := jcb481ServerWithSoftware(t)
	id, csrf, _, err := s.auth.Login(context.Background(), netip.MustParseAddr("127.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	// Build multipart manually: file part first, then reauth_password.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "ok.exe")
	content := append([]byte{0x4D, 0x5A}, bytes.Repeat([]byte("A"), 8)...)
	fw.Write(content)
	mw.Close()
	// append reauth field by reopening not possible; instead use WriteField before close? We need file first then field.
	// Rebuild correctly:
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("file", "ok.exe")
	fw.Write(content)
	mw.WriteField("reauth_password", "correct horse battery")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/software/staging", &buf)
	req.Host = "localhost"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: authCookieName(), Value: id})
	req.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", w.Code)
	}
}

func TestJCB481StagingOversizeReturns413(t *testing.T) {
	s := jcb481ServerWithSoftware(t)
	s.config.MaxUploadBytes = 16
	id, csrf, _, err := s.auth.Login(context.Background(), netip.MustParseAddr("127.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	content := append([]byte{0x4D, 0x5A}, bytes.Repeat([]byte("A"), 64)...)
	body, ct := jcb481MultipartBody(t, map[string]string{"reauth_password": "correct horse battery"}, content, "ok.exe")
	req := httptest.NewRequest(http.MethodPost, "https://localhost/api/v1/software/staging", body)
	req.Host = "localhost"
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("Content-Type", ct)
	req.AddCookie(&http.Cookie{Name: authCookieName(), Value: id})
	req.Header.Set("X-CSRF-Token", csrf)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d want 413", w.Code)
	}
	entries, _ := os.ReadDir(filepath.Join(s.config.DataDir, "staging"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Fatalf("residual part file: %s", e.Name())
		}
	}
}

func authCookieName() string { return auth.CookieName }
