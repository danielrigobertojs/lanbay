// SPDX-License-Identifier: Apache-2.0
// Command kitserver serves a short-lived M0 spike kit to one Windows machine.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
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

	"github.com/danielrigobertojs/lanbay/internal/auth"
)

const defaultMaxResultBytes int64 = 64 << 20

type server struct {
	token, kitPath, kitSHA256, resultsDir, bootstrapPath string
	lanbayURL, lanbayPassword, lanbayCertSHA256          string
	maxResultBytes                                       int64
	inactivity                                           time.Duration
	logger                                               *slog.Logger

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
	b, err := readBootstrapPath(s.bootstrapPath)
	if err != nil {
		http.Error(w, "bootstrap unavailable", http.StatusInternalServerError)
		return
	}
	baseURL := "http://" + r.Host + "/" + s.token
	replacer := strings.NewReplacer("__KIT_SHA256__", s.kitSHA256, "__KIT_URL__", baseURL+"/kit.zip", "__RESULTS_URL__", baseURL+"/results")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(replacer.Replace(string(b))))
}

func readBootstrap() ([]byte, error) { return readBootstrapPath("") }

func readBootstrapPath(path string) ([]byte, error) {
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	for _, candidate := range []string{
		"spikes/bootstrap.ps1", "../../spikes/bootstrap.ps1",
		"spikes/verify/bootstrap.ps1", "../../spikes/verify/bootstrap.ps1",
	} {
		if b, err := os.ReadFile(candidate); err == nil {
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
	var saved string
	if strings.HasPrefix(ct, "multipart/") {
		name := "results.zip"
		saved = filepath.Join(dir, name)
		if err := storeMultipart(dir, r.Body, r.Header.Get("Content-Type"), s.maxResultBytes); err != nil {
			return err
		}
		// storeMultipart writes its own filename; discover it.
		entries, _ := os.ReadDir(dir)
		if len(entries) > 0 {
			saved = filepath.Join(dir, entries[0].Name())
		}
	} else {
		name := filepath.Base(r.Header.Get("X-Lanbay-Result-Name"))
		if name == "." || name == "" {
			name = "results.zip"
		}
		saved = filepath.Join(dir, name)
		if err := copyResult(saved, r.Body); err != nil {
			return err
		}
	}
	// Extract and generate summary.md
	extractDir := filepath.Join(dir, "extracted")
	_ = extractResultsZip(saved, extractDir)
	_ = generateSummary(extractDir, s.lanbayURL, s.lanbayPassword, s.lanbayCertSHA256)
	_ = generateSummary(dir, s.lanbayURL, s.lanbayPassword, s.lanbayCertSHA256)
	return nil
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

func lanAddress() (*net.TCPAddr, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var fallback *net.TCPAddr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		v4, v6 := addressesForInterface(iface, addrs)
		if v4 != nil {
			return v4, nil
		}
		if fallback == nil && v6 != nil {
			fallback = v6
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, errors.New("no private or link-local LAN address found; pass -listen with one")
}

// addressesForInterface selects a private IPv4 address before IPv6. IPv6
// link-local addresses require an interface zone for a valid bind on macOS.
func addressesForInterface(iface net.Interface, addresses []net.Addr) (v4, v6 *net.TCPAddr) {
	for _, address := range addresses {
		ip, _, err := net.ParseCIDR(address.String())
		if err != nil || !privateOrLinkLocal(ip) || ip.IsLoopback() {
			continue
		}
		candidate := &net.TCPAddr{IP: ip}
		if ip.To4() != nil {
			return candidate, nil
		}
		if ip.IsLinkLocalUnicast() {
			candidate.Zone = iface.Name
		}
		if v6 == nil {
			v6 = candidate
		}
	}
	return nil, v6
}

func main() {
	kit := flag.String("kit", "dist/lanbay-m0-kit.zip", "path to kit zip")
	listen := flag.String("listen", ":8765", "private LAN address and port")
	results := flag.String("results", "m0-results", "result directory")
	idle := flag.Duration("idle-timeout", 30*time.Minute, "idle shutdown timeout")
	max := flag.Int64("max-result-bytes", defaultMaxResultBytes, "maximum result upload size")
	mode := flag.String("mode", "spike", "kit mode: spike or verify")
	bootstrap := flag.String("bootstrap", "", "path to bootstrap template")
	lanbayURL := flag.String("lanbay-url", "", "Lanbay base URL for verify login")
	lanbayPassword := flag.String("lanbay-password", "", "Lanbay password (or env LANBAY_PASSWORD)")
	lanbayCertSHA := flag.String("lanbay-cert-sha256", "", "SHA-256 of Lanbay TLS certificate (pinning)")
	verifyLogin := flag.Bool("verify-login", false, "perform Lanbay login + volume inventory and exit")
	summarize := flag.Bool("summarize", false, "generate summary.md from a results directory and exit")
	resultsDirFlag := flag.String("results-dir", "", "results directory for -summarize")
	flag.Parse()
	if *lanbayPassword == "" {
		*lanbayPassword = os.Getenv("LANBAY_PASSWORD")
	}
	if *idle <= 0 || *max <= 0 {
		slog.Error("idle-timeout and max-result-bytes must be positive")
		os.Exit(2)
	}
	if *summarize {
		dir := *resultsDirFlag
		if dir == "" {
			slog.Error("results-dir is required for -summarize")
			os.Exit(2)
		}
		if err := generateSummary(dir, *lanbayURL, *lanbayPassword, *lanbayCertSHA); err != nil {
			slog.Error("summarize", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if *verifyLogin {
		if *lanbayURL == "" || *lanbayPassword == "" {
			slog.Error("lanbay-url and lanbay-password are required for verify-login")
			os.Exit(2)
		}
		if err := runVerifyLogin(context.Background(), *lanbayURL, *lanbayPassword, *lanbayCertSHA); err != nil {
			slog.Error("verify login", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	addr, err := net.ResolveTCPAddr("tcp", *listen)
	if err != nil {
		slog.Error("invalid listen address", "error", err)
		os.Exit(2)
	}
	if addr.IP == nil || addr.IP.IsUnspecified() {
		selected, err := lanAddress()
		if err != nil {
			slog.Error("choose LAN address", "error", err)
			os.Exit(2)
		}
		addr.IP = selected.IP
		addr.Zone = selected.Zone
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
	s.bootstrapPath = *bootstrap
	if s.bootstrapPath == "" && *mode == "verify" {
		s.bootstrapPath = "spikes/verify/bootstrap.ps1"
	}
	s.lanbayURL = *lanbayURL
	s.lanbayPassword = *lanbayPassword
	s.lanbayCertSHA256 = *lanbayCertSHA
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	fmt.Printf("En PowerShell elevado de Windows, pegue:\nirm 'http://%s/%s/bootstrap.ps1' | iex\n", net.JoinHostPort(host, port), token)
	if err := s.run(context.Background(), listener); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}

// lanbayHTTPClient returns an HTTP client suitable for talking to a Lanbay
// instance, including its self-signed TLS certificate on the LAN.
func lanbayHTTPClient(baseURL, certSHA string) *http.Client {
	tr := &http.Transport{}
	if strings.HasPrefix(baseURL, "https://") {
		cfg := &tls.Config{InsecureSkipVerify: true}
		if certSHA != "" {
			cfg.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
				if len(rawCerts) == 0 {
					return fmt.Errorf("no peer certificate")
				}
				h := sha256.Sum256(rawCerts[0])
				if hex.EncodeToString(h[:]) != strings.ToLower(strings.TrimSpace(certSHA)) {
					return fmt.Errorf("certificate SHA-256 mismatch")
				}
				return nil
			}
		}
		tr.TLSClientConfig = cfg
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

func lanbayLogin(ctx context.Context, baseURL, password, certSHA string) (string, error) {
	client := lanbayHTTPClient(baseURL, certSHA)
	body, _ := json.Marshal(map[string]string{"password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName {
			return c.Value, nil
		}
	}
	// Fallback: parse Set-Cookie header manually.
	for _, h := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(h, auth.CookieName+"=") {
			parts := strings.SplitN(h, ";", 2)
			val := strings.TrimPrefix(parts[0], auth.CookieName+"=")
			if val != "" {
				return val, nil
			}
		}
	}
	return "", errors.New("session cookie not returned")
}

func lanbayVolumes(ctx context.Context, baseURL, sessionCookie, certSHA string) ([]map[string]any, error) {
	client := lanbayHTTPClient(baseURL, certSHA)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/volumes", nil)
	if err != nil {
		return nil, err
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: sessionCookie})
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("volumes failed: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var out struct {
		Volumes []map[string]any `json:"volumes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Volumes, nil
}

func runVerifyLogin(ctx context.Context, baseURL, password, certSHA string) error {
	cookie, err := lanbayLogin(ctx, baseURL, password, certSHA)
	if err != nil {
		return err
	}
	vols, err := lanbayVolumes(ctx, baseURL, cookie, certSHA)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(map[string]any{"ok": true, "volumes": vols})
}

type summaryCriterion struct {
	ID      string
	Section string
	Name    string
	Title   string
}

var summaryCriteria = []summaryCriterion{
	{"JCB-468", "M1", "icacls", "DACL de C:\\ProgramData\\Lanbay y initial-password.txt"},
	{"JCB-468", "M1", "junction-root", "Arranque con raíz como junction (debe negarse)"},
	{"JCB-468", "M1", "junction-internal", "Junction interna hacia System32 (debe negarse)"},
	{"JCB-468", "M1", "set-password-noecho", "set-password sin eco"},
	{"JCB-468", "M1", "disk-inventory", "Inventario de discos vía API"},
	{"JCB-468", "M1", "lanbay-login", "Login HTTPS desde la Mac"},
	{"JCB-476", "M4", "search", "Búsqueda winget"},
	{"JCB-476", "M4", "install", "Instalación (código 0/3010/1641)"},
	{"JCB-476", "M4", "update", "Actualización"},
	{"JCB-476", "M4", "uninstall", "Desinstalación gestionada"},
	{"JCB-476", "M4", "reconciliation-missing", "Reconciliación missing tras desinstalar fuera del panel"},
	{"JCB-476", "M4", "e3-preexisting-reject", "E3: app preexistente rechazada"},
	{"JCB-476", "M4", "sw1-appexeclink", "SW-1: resolución vía APPEXECLINK"},
	{"JCB-479", "M5", "b3-staging-dir", "B3: el producto crea staging"},
	{"JCB-479", "M5", "b2-path-outside-staging", "B2: ruta fuera de staging rechazada"},
	{"JCB-479", "M5", "authenticode", "Authenticode"},
	{"JCB-479", "M5", "msi-silent-install", "MSI silencioso"},
	{"JCB-479", "M5", "exe-interactive", "EXE interactivo (captura + confirmación)"},
	{"JCB-479", "M5", "registry-diff", "Diff del registro (0/1/más de 1)"},
	{"JCB-479", "M5", "uninstall", "Desinstalación de instaladores"},
}

func generateSummary(resultsDir, lanbayURL, password, certSHA string) error {
	results := map[string]map[string]any{}
	filepath.WalkDir(resultsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var v map[string]any
		if err := json.Unmarshal(b, &v); err != nil {
			return nil
		}
		rel, _ := filepath.Rel(resultsDir, path)
		key := strings.TrimSuffix(filepath.ToSlash(rel), ".json")
		results[key] = v
		return nil
	})

	var b strings.Builder
	b.WriteString("# Summary de verificación (JCB-485)\n\n")
	b.WriteString("| Criterio | Sección | Resultado | Evidencia |\n")
	b.WriteString("|---|---|---|---|\n")

	for _, c := range summaryCriteria {
		key := c.Section + "/" + c.Name
		status := "sin dato"
		evidence := ""
		if v, ok := results[key]; ok {
			if s, ok := v["status"].(string); ok && s != "" {
				status = s
			}
			if ev, ok := v["evidence"]; ok {
				raw, _ := json.Marshal(ev)
				evidence = string(raw)
				if len(evidence) > 120 {
					evidence = evidence[:120] + "…"
				}
			}
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.ID+" "+c.Title, c.Section, status, strings.ReplaceAll(evidence, "|", "\\|"))
	}

	// Login desde la Mac
	if lanbayURL != "" && password != "" {
		b.WriteString("\n## Login desde la Mac (JCB-468 paso 3)\n\n")
		cookie, err := lanbayLogin(context.Background(), lanbayURL, password, certSHA)
		if err != nil {
			fmt.Fprintf(&b, "- Resultado: **fail** (%s)\n", err.Error())
		} else {
			vols, err := lanbayVolumes(context.Background(), lanbayURL, cookie, certSHA)
			if err != nil {
				fmt.Fprintf(&b, "- Resultado: **fail** (%s)\n", err.Error())
			} else {
				b.WriteString("- Resultado: **pass**\n")
				fmt.Fprintf(&b, "- Volúmenes detectados: %d\n", len(vols))
			}
		}
	}

	out := filepath.Join(resultsDir, "summary.md")
	return os.WriteFile(out, []byte(b.String()), 0644)
}

const maxExtractEntries = 1000
const maxExtractBytes int64 = 256 << 20

func extractResultsZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	if len(r.File) > maxExtractEntries {
		return fmt.Errorf("too many entries: %d", len(r.File))
	}
	var total int64
	for _, f := range r.File {
		if !filepath.IsLocal(f.Name) {
			return fmt.Errorf("unsafe path in zip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.FileInfo().Mode().IsRegular() {
			return fmt.Errorf("non-regular file in zip: %s", f.Name)
		}
		target := filepath.Join(dest, f.Name)
		rel, err := filepath.Rel(dest, target)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe path in zip: %s", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			rc.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(rc, maxExtractBytes-total+1))
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		total += n
		if total > maxExtractBytes {
			return fmt.Errorf("extracted size exceeds limit")
		}
	}
	return nil
}
