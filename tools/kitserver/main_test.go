// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testServer(t *testing.T, limit int64) *server {
	t.Helper()
	root := t.TempDir()
	kit := filepath.Join(root, "kit.zip")
	if err := os.WriteFile(kit, []byte("kit bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := newServer("secret", kit, filepath.Join(root, "results"), limit, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func request(s *server, method, path string, body []byte, remote string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.RemoteAddr = remote
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestTokenAndLANRestrictions(t *testing.T) {
	s := testServer(t, 1024)
	if got := request(s, http.MethodGet, "/bootstrap.ps1", nil, "192.168.1.10:1234").Code; got != http.StatusNotFound {
		t.Fatalf("without token: %d", got)
	}
	if got := request(s, http.MethodGet, "/wrong/bootstrap.ps1", nil, "192.168.1.10:1234").Code; got != http.StatusNotFound {
		t.Fatalf("wrong token: %d", got)
	}
	if got := request(s, http.MethodGet, "/secret/bootstrap.ps1", nil, "8.8.8.8:1234").Code; got != http.StatusForbidden {
		t.Fatalf("public address: %d", got)
	}
	if got := request(s, http.MethodGet, "/secret/bootstrap.ps1", nil, "192.168.1.10:1234").Code; got != http.StatusOK {
		t.Fatalf("private address: %d", got)
	}
}

func TestBootstrapPinsKitHash(t *testing.T) {
	s := testServer(t, 1024)
	w := request(s, http.MethodGet, "/secret/bootstrap.ps1", nil, "127.0.0.1:1234")
	if !bytes.Contains(w.Body.Bytes(), []byte(s.kitSHA256)) {
		t.Fatal("bootstrap does not contain kit SHA-256")
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("http://example.com/secret/kit.zip")) {
		t.Fatal("bootstrap does not contain an absolute kit URL")
	}
	if bytes.Contains(w.Body.Bytes(), []byte("__KIT_SHA256__")) {
		t.Fatal("unsubstituted hash placeholder")
	}
}

func TestBootstrapAbortsOnHashMismatchBeforeExtraction(t *testing.T) {
	b, err := readBootstrap()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("if ($actualHash -ne $kitHash) { throw")) {
		t.Fatal("bootstrap lacks a fail-closed hash mismatch guard")
	}
	if bytes.Index(b, []byte("if ($actualHash -ne $kitHash) { throw")) > bytes.Index(b, []byte("Expand-Archive")) {
		t.Fatal("bootstrap extracts before checking the kit hash")
	}
}

func TestResultsStoredUnmodified(t *testing.T) {
	s := testServer(t, 1024)
	body := []byte{0, 1, 2, 3, 255, 0}
	w := request(s, http.MethodPost, "/secret/results", body, "127.0.0.1:1234")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	entries, err := os.ReadDir(s.resultsDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("result directories: %v %d", err, len(entries))
	}
	stored, err := os.ReadFile(filepath.Join(s.resultsDir, entries[0].Name(), "results.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, body) {
		t.Fatalf("results modified: %x != %x", stored, body)
	}
}

func TestResultsOverLimitRejected(t *testing.T) {
	s := testServer(t, 3)
	w := request(s, http.MethodPost, "/secret/results", []byte("four"), "127.0.0.1:1234")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestRunStopsAfterInactivity(t *testing.T) {
	s := testServer(t, 1024)
	s.inactivity = 20 * time.Millisecond
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	finished := make(chan error, 1)
	go func() { finished <- s.run(context.Background(), listener) }()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop after inactivity")
	}
}
