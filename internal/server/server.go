// SPDX-License-Identifier: Apache-2.0
// Package server exposes the TLS-only local Lanbay HTTP API.
package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danielrigobertojs/lanbay/internal/auth"
	"github.com/danielrigobertojs/lanbay/internal/config"
	"github.com/danielrigobertojs/lanbay/internal/files"
	"github.com/danielrigobertojs/lanbay/internal/jobs"
	"github.com/danielrigobertojs/lanbay/internal/pathguard"
	"github.com/danielrigobertojs/lanbay/internal/platform"
	"github.com/danielrigobertojs/lanbay/internal/software"
	"github.com/danielrigobertojs/lanbay/internal/uploads"
	"github.com/danielrigobertojs/lanbay/internal/volumes"
	lanbayweb "github.com/danielrigobertojs/lanbay/internal/web"
	"io"
	"log/slog"
	"mime/multipart"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	config   config.Config
	auth     *auth.Manager
	volumes  volumes.Provider
	db       *sql.DB
	version  string
	auditMu  sync.Mutex
	jobs     *jobs.Manager
	uploads  *uploads.Manager
	software *software.Service
}

func New(c config.Config, db *sql.DB, provider volumes.Provider, version string) *Server {
	s := &Server{config: c, auth: auth.New(db, c.SessionIdle), volumes: provider, db: db, version: version}
	if db != nil {
		s.jobs, _ = jobs.New(db)
		if s.jobs != nil {
			s.uploads = uploads.New(db, s.jobs)
		}
	}
	return s
}
func (s *Server) SetSoftware(svc *software.Service)              { s.software = svc }
func (s *Server) Jobs() *jobs.Manager                            { return s.jobs }
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
	mux.HandleFunc("POST /api/v1/uploads", s.uploadCreate)
	mux.HandleFunc("PATCH /api/v1/uploads/{id}", s.uploadChunk)
	mux.HandleFunc("DELETE /api/v1/uploads/{id}", s.uploadCancel)
	mux.HandleFunc("GET /api/v1/jobs", s.jobList)
	mux.HandleFunc("POST /api/v1/jobs/{id}/cancel", s.jobCancel)
	mux.HandleFunc("GET /api/v1/software", s.softwareList)
	mux.HandleFunc("GET /api/v1/software/search", s.softwareSearch)
	mux.HandleFunc("POST /api/v1/software/install", s.softwareInstall)
	mux.HandleFunc("POST /api/v1/software/install-uploaded", s.softwareInstallUploaded)
	mux.HandleFunc("GET /api/v1/software/installers/inspect", s.softwareInspectInstaller)
	mux.HandleFunc("GET /api/v1/software/installers/pending/{job}", s.softwarePendingChoices)
	mux.HandleFunc("POST /api/v1/software/installers/confirm", s.softwareConfirmInstalled)
	mux.HandleFunc("POST /api/v1/software/staging", s.softwareStageInstaller)
	mux.HandleFunc("GET /api/v1/software/updates", s.softwareUpdates)
	mux.HandleFunc("POST /api/v1/software/{id}/upgrade", s.softwareUpgrade)
	mux.HandleFunc("POST /api/v1/software/{id}/uninstall", s.softwareUninstall)
	mux.HandleFunc("GET /api/v1/events", s.events)
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
	protected, err := platform.ProtectedPaths(s.config.DataDir, s.config.ProtectedPaths)
	if err != nil {
		return files.Manager{}, err
	}
	return files.Manager{Guard: pathguard.Guard{Roots: roots, Protected: protected}, Trash: files.NewTrash()}, nil
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
	if e == nil && s.jobs == nil {
		e = errors.New("jobs no disponibles")
	}
	if e != nil {
		fileError(w, e)
		return
	}
	kind := "copy"
	if move {
		kind = "move"
	}
	j, e := s.jobs.Create(r.Context(), kind, 0)
	if e != nil {
		http.Error(w, "no se pudo crear el job", 500)
		return
	}
	s.jobs.Start(j, func(ctx context.Context) error {
		if move {
			return m.Move(ctx, in.Source, in.Parent, in.Name)
		}
		return m.Copy(ctx, in.Source, in.Parent, in.Name)
	})
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(j)
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
func (s *Server) uploadCreate(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.uploads == nil {
		http.Error(w, "subidas no disponibles", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Parent string `json:"parent"`
		Name   string `json:"name"`
		Size   int64  `json:"size_bytes"`
		Chunk  int64  `json:"chunk_size"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	m, err := s.fileManager(r.Context())
	if err == nil {
		session, createErr := s.uploads.Create(r.Context(), m.Guard, in.Parent, in.Name, in.Size, in.Chunk)
		err = createErr
		if err == nil {
			w.Header().Set("Location", "/api/v1/uploads/"+session.ID)
			w.Header().Set("Upload-Offset", "0")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(session)
			return
		}
	}
	fileError(w, err)
}
func (s *Server) uploadChunk(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.uploads == nil {
		http.Error(w, "subidas no disponibles", http.StatusServiceUnavailable)
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset < 0 || r.ContentLength <= 0 || r.ContentLength > uploads.MaxChunk {
		http.Error(w, "chunk inválido", 400)
		return
	}
	session, err := s.uploads.Write(r.Context(), r.PathValue("id"), offset, r.ContentLength, http.MaxBytesReader(w, r.Body, uploads.MaxChunk))
	if err != nil {
		fileError(w, err)
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(session.Offset, 10))
	w.Header().Set("Upload-State", session.State)
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) uploadCancel(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.uploads == nil {
		http.Error(w, "subidas no disponibles", http.StatusServiceUnavailable)
		return
	}
	if err := s.uploads.Cancel(r.Context(), r.PathValue("id")); err != nil {
		fileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) jobList(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.jobs == nil {
		http.Error(w, "jobs no disponibles", http.StatusServiceUnavailable)
		return
	}
	items, err := s.jobs.List(r.Context())
	if err != nil {
		http.Error(w, "no se pudieron leer los jobs", 500)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jobs": items})
}
func (s *Server) jobCancel(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.jobs == nil {
		http.Error(w, "jobs no disponibles", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	var uploadID string
	err := s.db.QueryRowContext(r.Context(), "SELECT id FROM uploads WHERE job_id=? AND state='active'", id).Scan(&uploadID)
	if err == nil {
		err = s.uploads.Cancel(r.Context(), uploadID)
	} else if errors.Is(err, sql.ErrNoRows) {
		err = s.jobs.Cancel(r.Context(), id)
	}
	if err != nil {
		fileError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.jobs == nil {
		http.Error(w, "jobs no disponibles", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming no disponible", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	events, stop := s.jobs.Subscribe()
	defer stop()
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "event: job\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
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
	verified, failed, e := s.auth.VerifyPasswordForSession(r.Context(), cookie.Value, in.CurrentPassword)
	if e != nil {
		if failed {
			s.audit(r, "password_change", "failed")
		}
		http.Error(w, "no se pudo revocar la sesión", http.StatusInternalServerError)
		return
	}
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
func (s *Server) softwareList(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	items, err := s.software.List(r.Context())
	if err != nil {
		http.Error(w, "no se pudo leer el software", 500)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"apps": items, "restart_pending": s.software.RestartPending()})
}
func (s *Server) softwareSearch(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query().Get("q")
	res, err := s.software.Search(r.Context(), q)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, software.ErrDisabled) || errors.Is(err, software.ErrUnavailable) {
			code = http.StatusServiceUnavailable
		}
		if errors.Is(err, software.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"results": res})
}
func (s *Server) softwareInstall(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Source string `json:"source"`
		ID     string `json:"id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	if in.Source != "" && in.Source != string(software.SourceWinget) {
		http.Error(w, "source no soportado en M4", 400)
		return
	}
	id := in.ID
	if id == "" {
		http.Error(w, "id requerido", 400)
		return
	}
	jobID, err := s.software.InstallWinget(r.Context(), id)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, software.ErrDisabled) || errors.Is(err, software.ErrUnavailable) {
			code = http.StatusServiceUnavailable
		}
		if errors.Is(err, software.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	s.audit(r, "software_install:"+id, "ok")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}
func (s *Server) softwareUpdates(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	items, err := s.software.Updates(r.Context())
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, software.ErrDisabled) || errors.Is(err, software.ErrUnavailable) {
			code = http.StatusServiceUnavailable
		}
		if errors.Is(err, software.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"updates": items})
}
func (s *Server) softwareUpgrade(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	jobID, err := s.software.Upgrade(r.Context(), id)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, software.ErrDisabled) || errors.Is(err, software.ErrUnavailable) {
			code = http.StatusServiceUnavailable
		}
		if errors.Is(err, software.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	s.audit(r, "software_upgrade:"+id, "ok")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}
func (s *Server) softwareUninstall(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	confirm := r.URL.Query().Get("confirm") == "1"
	jobID, err := s.software.Uninstall(r.Context(), id, confirm)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, software.ErrDisabled) || errors.Is(err, software.ErrUnavailable) {
			code = http.StatusServiceUnavailable
		}
		if errors.Is(err, software.ErrNotFound) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	s.audit(r, "software_uninstall:"+id, "ok")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}
func (s *Server) softwareInstallUploaded(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Path            string   `json:"path"`
		Name            string   `json:"name"`
		InstallerID     string   `json:"installer_id"`
		Preset          string   `json:"preset"`
		Args            []string `json:"args"`
		Interactive     bool     `json:"interactive"`
		Keep            bool     `json:"keep_installer"`
		ConfirmUnsigned bool     `json:"confirm_unsigned"`
		ConfirmName     string   `json:"confirm_name"`
		ReauthPassword  string   `json:"reauth_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	cookie, _ := r.Cookie(auth.CookieName)
	verified, failed, e := s.auth.VerifyPasswordForSession(r.Context(), cookie.Value, in.ReauthPassword)
	if e != nil {
		if failed {
			s.audit(r, "installer_install", "failed")
		}
		http.Error(w, "no se pudo verificar la sesión", http.StatusInternalServerError)
		return
	}
	if !verified {
		if failed {
			s.audit(r, "installer_install", "failed")
		}
		http.Error(w, "re-autenticación requerida", http.StatusUnauthorized)
		return
	}
	if in.Path != "" {
		in.Name = filepath.Base(in.Path)
		in.Path = ""
	}
	jobID, err := s.software.InstallUploaded(r.Context(), software.InstallUploadOptions{
		Path: in.Path, Name: in.Name, InstallerID: in.InstallerID, Preset: in.Preset, Args: in.Args, Interactive: in.Interactive, KeepInstaller: in.Keep, ConfirmUnsigned: in.ConfirmUnsigned, ConfirmName: in.ConfirmName,
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
	s.audit(r, "installer_install", "ok")

}

func (s *Server) softwareInspectInstaller(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = filepath.Base(r.URL.Query().Get("path"))
	}
	info, err := s.software.InspectInstaller(name)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	_ = json.NewEncoder(w).Encode(info)
}

func (s *Server) softwarePendingChoices(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, false) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	job := r.PathValue("job")
	_ = json.NewEncoder(w).Encode(map[string]any{"choices": s.software.PendingInstallChoices(job)})
}

func (s *Server) softwareConfirmInstalled(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	var in struct {
		Job  string   `json:"job_id"`
		Keys []string `json:"keys"`
		Keep bool     `json:"keep_installer"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, "solicitud inválida", 400)
		return
	}
	if err := s.software.ConfirmInstalled(r.Context(), in.Job, in.Keys, in.Keep); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) softwareStageInstaller(w http.ResponseWriter, r *http.Request) {
	if !s.session(w, r, true) {
		return
	}
	if s.software == nil {
		http.Error(w, "software no disponible", http.StatusServiceUnavailable)
		return
	}
	limit := s.config.MaxUploadBytes
	if limit <= 0 {
		limit = 128 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	mr, err := r.MultipartReader()
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "solicitud demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "solicitud inválida", 400)
		return
	}
	// Read reauth_password first.
	var reauth string
	var filePart *multipart.Part
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				if filePart != nil {
					filePart.Close()
				}
				http.Error(w, "solicitud demasiado grande", http.StatusRequestEntityTooLarge)
				return
			}
			if filePart != nil {
				filePart.Close()
			}
			http.Error(w, "solicitud inválida", 400)
			return
		}
		if part.FormName() == "reauth_password" {
			b, _ := io.ReadAll(io.LimitReader(part, 1024))
			reauth = string(b)
			part.Close()
			continue
		}
		if part.FormName() == "file" {
			filePart = part
			break
		}
		part.Close()
	}
	cookie, _ := r.Cookie(auth.CookieName)
	verified, failed, e := s.auth.VerifyPasswordForSession(r.Context(), cookie.Value, reauth)
	if e != nil || !verified {
		if failed {
			s.audit(r, "staging_upload", "failed")
		}
		if filePart != nil {
			filePart.Close()
		}
		http.Error(w, "re-autenticación requerida", http.StatusUnauthorized)
		return
	}
	if filePart == nil {
		http.Error(w, "archivo requerido", 400)
		return
	}
	defer filePart.Close()
	name, err := s.software.StageInstallerStream(filePart.FileName(), filePart)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "solicitud demasiado grande", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, err.Error(), 400)
		return
	}
	s.audit(r, "staging_upload", "ok")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"name": name})
}

func (s *Server) audit(r *http.Request, action, result string) {
	// SQLite accepts one writer at a time. Audit records are security evidence,
	// so concurrent requests must not silently lose one to a busy writer.
	// Use a context detached from the request so a client that closes the
	// connection does not drop the audit INSERT.
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	ctx := context.WithoutCancel(r.Context())
	if _, err := s.db.ExecContext(ctx, "INSERT INTO audit_log(created_at,ip,action,result) VALUES(?,?,?,?)", time.Now().UTC().Format(time.RFC3339), remoteIP(r).String(), action, result); err != nil {
		slog.Error("write audit record", "action", action, "result", result, "error", err)
	}
}
