// SPDX-License-Identifier: Apache-2.0
// Package server exposes the TLS-only local Lanbay HTTP API.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/danielrigobertojs/lanbay/internal/auth"
	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/volumes"
	lanbayweb "github.com/danielrigobertojs/lanbay/internal/web"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
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
	mux.Handle("GET /", lanbayweb.Handler())
	return s.network(s.headers(s.log(mux)))
}

func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
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
	ok, change := s.auth.Check(cookie.Value, r.Header.Get("X-CSRF-Token"), write)
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
		Password string `json:"password"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	ip := remoteIP(r)
	id, csrf, change, e := s.auth.Login(r.Context(), ip, in.Password)
	if e != nil {
		s.audit(r, "login", "failed")
		http.Error(w, "credenciales no válidas", http.StatusUnauthorized)
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
	s.auth.Logout(c.Value)
	http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) password(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	if e := s.SetPassword(r.Context(), in.Password); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	cookie, _ := r.Cookie(auth.CookieName)
	s.auth.CompletePasswordChange(cookie.Value)
	s.auth.RevokeOtherSessions(cookie.Value)
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
