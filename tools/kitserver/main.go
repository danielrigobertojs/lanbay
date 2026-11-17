// SPDX-License-Identifier: Apache-2.0
// Command kitserver serves a short-lived M0 spike kit to one Windows machine.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultMaxResultBytes int64 = 64 << 20

type server struct {
	token, kitPath, kitSHA256, resultsDir string
	maxResultBytes                        int64
	inactivity                            time.Duration
	logger                                *slog.Logger

	mu       sync.Mutex
	lastUsed time.Time
	done     chan struct{}
	doneOnce sync.Once
}

func newServer(token, kitPath, resultsDir string, maxResultBytes int64, inactivity time.Duration, logger *slog.Logger) (*server, error) {
	f, err := os.Open(kitPath)
	if err != nil {
		return nil, fmt.Errorf("open kit: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("hash kit: %w", err)
	}
	return &server{token: token, kitPath: kitPath, kitSHA256: hex.EncodeToString(h.Sum(nil)), resultsDir: resultsDir, maxResultBytes: maxResultBytes, inactivity: inactivity, logger: logger, lastUsed: time.Now(), done: make(chan struct{})}, nil
}

func (s *server) closeDone() { s.doneOnce.Do(func() { close(s.done) }) }
func (s *server) touch()     { s.mu.Lock(); s.lastUsed = time.Now(); s.mu.Unlock() }
func (s *server) idleFor() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastUsed)
}

func privateOrLinkLocal(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	return ip.IsLoopback()
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !privateOrLinkLocal(net.ParseIP(ip)) {
		http.Error(w, "LAN clients only", http.StatusForbidden)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != s.token {
		http.NotFound(w, r)
		return
	}
	s.touch()
	switch {
	case r.Method == http.MethodGet && parts[1] == "bootstrap.ps1":
		s.serveBootstrap(w, r)
	case r.Method == http.MethodGet && parts[1] == "kit.zip":
		http.ServeFile(w, r, s.kitPath)
	case r.Method == http.MethodPost && parts[1] == "results":
		if err := s.storeResults(r); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
		s.closeDone()
	default:
		http.NotFound(w, r)
	}
}

func (s *server) serveBootstrap(w http.ResponseWriter, r *http.Request) {
	b, err := readBootstrap()
	if err != nil {
		http.Error(w, "bootstrap unavailable", http.StatusInternalServerError)
		return
	}
	baseURL := "http://" + r.Host + "/" + s.token
	replacer := strings.NewReplacer("__KIT_SHA256__", s.kitSHA256, "__KIT_URL__", baseURL+"/kit.zip", "__RESULTS_URL__", baseURL+"/results")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(replacer.Replace(string(b))))
}

func readBootstrap() ([]byte, error) {
	for _, path := range []string{"spikes/bootstrap.ps1", "../../spikes/bootstrap.ps1"} {
		if b, err := os.ReadFile(path); err == nil {
			return b, nil
		}
	}
	return nil, errors.New("bootstrap template not found")
}

func (s *server) storeResults(r *http.Request) error {
	r.Body = http.MaxBytesReader(nil, r.Body, s.maxResultBytes)
	dir := filepath.Join(s.resultsDir, time.Now().UTC().Format("20060102T150405.000000000Z"))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create result directory: %w", err)
	}
	defer r.Body.Close()
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if strings.HasPrefix(ct, "multipart/") {
		return storeMultipart(dir, r.Body, r.Header.Get("Content-Type"), s.maxResultBytes)
	}
	name := filepath.Base(r.Header.Get("X-Lanbay-Result-Name"))
	if name == "." || name == "" {
		name = "results.zip"
	}
	return copyResult(filepath.Join(dir, name), r.Body)
}

func storeMultipart(dir string, body io.Reader, contentType string, limit int64) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("parse multipart: %w", err)
	}
	mr := multipart.NewReader(body, params["boundary"])
	part, err := mr.NextPart()
	if err != nil {
		return fmt.Errorf("read multipart: %w", err)
	}
	defer part.Close()
	name := filepath.Base(part.FileName())
	if name == "." || name == "" {
		name = "results.zip"
	}
	return copyResult(filepath.Join(dir, name), io.LimitReader(part, limit+1))
}

func copyResult(path string, src io.Reader) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create result: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, src); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}

func (s *server) run(ctx context.Context, listener net.Listener) error {
	httpServer := &http.Server{Handler: s}
	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.Serve(listener) }()
	ticker := time.NewTicker(minDuration(s.inactivity/4, time.Second))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = httpServer.Shutdown(context.Background())
			return ctx.Err()
		case <-s.done:
			return httpServer.Shutdown(context.Background())
		case <-ticker.C:
			if s.idleFor() >= s.inactivity {
				return httpServer.Shutdown(context.Background())
			}
		case err := <-errCh:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		}
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func lanAddress() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addrs {
			ip, _, err := net.ParseCIDR(address.String())
			if err == nil && privateOrLinkLocal(ip) && !ip.IsLoopback() {
				return ip, nil
			}
		}
	}
	return nil, errors.New("no private or link-local LAN address found; pass -listen with one")
}

func main() {
	kit := flag.String("kit", "dist/lanbay-m0-kit.zip", "path to kit zip")
	listen := flag.String("listen", ":8765", "private LAN address and port")
	results := flag.String("results", "m0-results", "result directory")
	idle := flag.Duration("idle-timeout", 30*time.Minute, "idle shutdown timeout")
	max := flag.Int64("max-result-bytes", defaultMaxResultBytes, "maximum result upload size")
	flag.Parse()
	if *idle <= 0 || *max <= 0 {
		slog.Error("idle-timeout and max-result-bytes must be positive")
		os.Exit(2)
	}
	addr, err := net.ResolveTCPAddr("tcp", *listen)
	if err != nil {
		slog.Error("invalid listen address", "error", err)
		os.Exit(2)
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		addr.IP, err = lanAddress()
		if err != nil {
			slog.Error("choose LAN address", "error", err)
			os.Exit(2)
		}
	}
	if !privateOrLinkLocal(addr.IP) {
		slog.Error("listen address must be private, link-local, or loopback")
		os.Exit(2)
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		slog.Error("listen", "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	token, err := randomToken()
	if err != nil {
		slog.Error("token", "error", err)
		os.Exit(1)
	}
	s, err := newServer(token, *kit, *results, *max, *idle, slog.Default())
	if err != nil {
		slog.Error("kit", "error", err)
		os.Exit(1)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	fmt.Printf("En PowerShell elevado de Windows, pegue:\nirm 'http://%s/%s/bootstrap.ps1' | iex\n", net.JoinHostPort(host, port), token)
	if err := s.run(context.Background(), listener); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
