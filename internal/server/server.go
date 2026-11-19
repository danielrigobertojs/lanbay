// SPDX-License-Identifier: Apache-2.0
// Package server exposes the TLS-only local Lanbay HTTP API.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/danielrigobertojs/lanbay/internal/auth"
	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/files"
	"github.com/danielrigobertojs/lanbay/internal/pathguard"
	"github.com/danielrigobertojs/lanbay/internal/volumes"
	lanbayweb "github.com/danielrigobertojs/lanbay/internal/web"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	config  config.Config
	auth    *auth.Manager
	volumes volumes.Provider
	db      *sql.DB
	version string
}

func New(c config.Config, db *sql.DB, provider volumes.Provider, version string) *Server {
	return &Server{config: c, auth: auth.New(db, c.SessionIdle), volumes: provider, db: db, version: version}
}
func (s *Server) Initialize(ctx context.Context) (string, error) { return s.auth.EnsurePassword(ctx) }
func (s *Server) SetPassword(ctx context.Context, password string) error {
	if err := s.auth.SetPassword(ctx, password); err != nil {
		return err
	}
	err := os.Remove(filepath.Join(s.config.DataDir, "initial-password.txt"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
func AllowedIP(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("POST /api/v1/auth/password", s.password)
	mux.HandleFunc("GET /api/v1/system", s.system)
	mux.HandleFunc("GET /api/v1/volumes", s.volumeList)
	mux.HandleFunc("GET /api/v1/files", s.fileList)
	mux.HandleFunc("POST /api/v1/files/mkdir", s.fileMkdir)
	mux.HandleFunc("POST /api/v1/files/rename", s.fileRename)
	mux.HandleFunc("POST /api/v1/files/move", s.fileMove)
	mux.HandleFunc("POST /api/v1/files/copy", s.fileCopy)
	mux.HandleFunc("DELETE /api/v1/files", s.fileDelete)
	mux.HandleFunc("GET /api/v1/files/download", s.fileDownload)
	mux.Handle("GET /", lanbayweb.Handler())
	return s.network(s.host(s.headers(s.log(mux))))
}
func (s *Server) host(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if host == "" || !allowedHost(host, slices.Concat(s.config.AllowedHosts, config.RuntimeHosts())) {
			http.Error(w, "host no permitido", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func allowedHost(host string, allowed []string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	for _, item := range allowed {
		if host == strings.Trim(strings.ToLower(item), "[]") {
			return true
		}
	}
	return false
}
func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'self'; media-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			origin := r.Header.Get("Origin")
			if origin != "" && origin != "https://"+r.Host {
				http.Error(w, "origen no permitido", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) fileManager(ctx context.Context) (files.Manager, error) {
	vs, err := s.volumes.List(ctx)
	if err != nil {
		return files.Manager{}, err
	}
	roots := make([]string, 0, len(vs))
	for _, v := range vs {
		roots = append(roots, v.ID)
	}
	return files.Manager{Guard: pathguard.Guard{Roots: roots, Protected: []string{s.config.DataDir}}, Trash: files.NewTrash()}, nil
}
func (s *Server) fileList(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	m, e := s.fileManager(r.Context())
	if e != nil {
		http.Error(w, "no se pudieron enumerar los volúmenes", 500)
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	per, _ := strconv.Atoi(q.Get("per_page"))
	out, e := m.List(r.Context(), q.Get("path"), files.ListOptions{Page: page, PerPage: per, Sort: q.Get("sort"), Hidden: q.Get("hidden") == "true", System: q.Get("system") == "true"})
	if e != nil {
		fileError(w, e)
		return
	}
	json.NewEncoder(w).Encode(out)
}
func (s *Server) fileMkdir(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	var in struct{ Parent, Name string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	m, e := s.fileManager(r.Context())
	if e == nil {
		_, e = m.Mkdir(r.Context(), in.Parent, in.Name)
	}
	if e != nil {
		fileError(w, e)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
func (s *Server) fileRename(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	var in struct{ Path, Name string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	m, e := s.fileManager(r.Context())
	if e == nil {
		_, e = m.Rename(r.Context(), in.Path, in.Name)
	}
	if e != nil {
		fileError(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) fileMove(w http.ResponseWriter, r *http.Request) { s.fileTransfer(w, r, true) }
func (s *Server) fileCopy(w http.ResponseWriter, r *http.Request) { s.fileTransfer(w, r, false) }
func (s *Server) fileTransfer(w http.ResponseWriter, r *http.Request, move bool) {
	if !s.session(w, r, true) {
		return
	}
	var in struct{ Source, Parent, Name string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	m, e := s.fileManager(r.Context())
	if e == nil {
		if move {
			e = m.Move(r.Context(), in.Source, in.Parent, in.Name)
		} else {
			e = m.Copy(r.Context(), in.Source, in.Parent, in.Name)
		}
	}
	if e != nil {
		fileError(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) fileDelete(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	var in struct {
		Path      string
		Permanent bool `json:"permanent"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	m, e := s.fileManager(r.Context())
	if e == nil {
		volumes, listErr := s.volumes.List(r.Context())
		if listErr != nil {
			e = listErr
		} else {
			e = m.Delete(r.Context(), in.Path, files.DeleteOptions{Permanent: in.Permanent, Recycle: recycleAvailable(in.Path, volumes)})
		}
	}
	if e != nil {
		fileError(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func recycleAvailable(path string, items []volumes.Volume) bool {
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(strings.ReplaceAll(path, "/", "\\")), strings.ToLower(item.ID)) {
			return item.Recycle
		}
	}
	return false
}
func (s *Server) fileDownload(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	m, err := s.fileManager(r.Context())
	if err != nil {
		http.Error(w, "no se pudieron enumerar los volúmenes", 500)
		return
	}
	path := r.URL.Query().Get("path")
	if r.URL.Query().Get("zip") == "true" {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="lanbay-download.zip"`)
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
		if err := m.Zip(r.Context(), path, w); err != nil {
			fileError(w, err)
		}
		return
	}
	f, info, err := m.Open(path)
	if err != nil {
		fileError(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(filepath.Base(info.Name())))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
func fileError(w http.ResponseWriter, e error) {
	code := http.StatusBadRequest
	if errors.Is(e, pathguard.ErrProtected) || errors.Is(e, pathguard.ErrRoot) {
		code = http.StatusForbidden
	}
	http.Error(w, e.Error(), code)
}
func (s *Server) network(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		ip, e2 := netip.ParseAddr(host)
		if e != nil || e2 != nil || !AllowedIP(ip.Unmap(), s.config.AllowedNetworks) {
			http.Error(w, "origen de red no permitido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("http request", "method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
func (s *Server) session(w http.ResponseWriter, r *http.Request, write bool) bool {
	cookie, e := r.Cookie(auth.CookieName)
	if e != nil {
		http.Error(w, "autenticación requerida", http.StatusUnauthorized)
		return false
	}
	ok, change := s.auth.Check(r.Context(), cookie.Value, r.Header.Get("X-CSRF-Token"), write)
	if !ok {
		http.Error(w, "sesión no válida o CSRF inválido", http.StatusUnauthorized)
		return false
	}
	if change && r.URL.Path != "/api/v1/auth/password" {
		http.Error(w, "debe cambiar la contraseña inicial", http.StatusForbidden)
		return false
	}
	return true
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password        string `json:"password"`
		CurrentPassword string `json:"current_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "solicitud demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "solicitud inválida", 400)
		return
	}
	ip := remoteIP(r)
	id, csrf, change, e := s.auth.Login(r.Context(), ip, in.Password)
	if e != nil {
		s.audit(r, "login", "failed")
		http.Error(w, e.Error(), http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: id, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: int(s.config.SessionIdle.Seconds())})
	s.audit(r, "login", "ok")
	json.NewEncoder(w).Encode(map[string]any{"csrf_token": csrf, "password_change_required": change})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	c, _ := r.Cookie(auth.CookieName)
	s.auth.Logout(r.Context(), c.Value)
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	var in struct {
		Password        string `json:"password"`
		CurrentPassword string `json:"current_password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "solicitud demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "solicitud inválida", 400)
		return
	}
	cookie, _ := r.Cookie(auth.CookieName)
	verified, failed := s.auth.VerifyPasswordForSession(r.Context(), cookie.Value, in.CurrentPassword)
	if !verified {
		if failed {
			s.audit(r, "password_change", "failed")
		}
		http.Error(w, "credenciales no válidas", http.StatusUnauthorized)
		return
	}
	changed, e := s.auth.ChangePasswordForSession(r.Context(), cookie.Value, in.Password)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if !changed {
		http.Error(w, "sesión no válida o CSRF inválido", http.StatusUnauthorized)
		return
	}
	if e := os.Remove(filepath.Join(s.config.DataDir, "initial-password.txt")); e != nil && !os.IsNotExist(e) {
		http.Error(w, e.Error(), http.StatusBadRequest)
		return
	}
	s.audit(r, "password_change", "ok")
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"hostname": hostname(), "version": s.version, "platform": "desarrollo"})
}
func (s *Server) volumeList(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	items, e := s.volumes.List(r.Context())
	if e != nil {
		http.Error(w, "no se pudieron enumerar los volúmenes", 500)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"volumes": items, "alert_percent": s.config.DiskAlertPercent})
}
func remoteIP(r *http.Request) netip.Addr {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return netip.MustParseAddr(strings.Trim(host, "[]"))
}
func hostname() string { h, _ := os.Hostname(); return h }
func (s *Server) audit(r *http.Request, action, result string) {
	_, _ = s.db.ExecContext(r.Context(), "INSERT INTO audit_log(created_at,ip,action,result) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339), remoteIP(r).String(), action, result)
}
