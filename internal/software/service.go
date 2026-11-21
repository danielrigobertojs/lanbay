// SPDX-License-Identifier: Apache-2.0
package software

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
)

type Service struct {
	db       *sql.DB
	Registry *Registry
	Backend  Backend
	Jobs     *jobs.Manager
	DataDir  string

	mu             sync.Mutex
	restartPending bool
	installMu      sync.Mutex
}

var wingetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)

func validateWingetID(id string) error {
	if !wingetIDPattern.MatchString(id) {
		return fmt.Errorf("id de winget inválido")
	}
	return nil
}

func validateSearchQuery(q string) error {
	if len(q) > 128 {
		return fmt.Errorf("búsqueda demasiado larga")
	}
	if len(q) > 0 && q[0] == '-' {
		return fmt.Errorf("búsqueda inválida")
	}
	for _, r := range q {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("búsqueda inválida")
		}
	}
	return nil
}

func NewService(db *sql.DB, backend Backend, jm *jobs.Manager, dataDir string) (*Service, error) {
	if err := EnsureSchema(context.Background(), db); err != nil {
		return nil, err
	}
	return &Service{db: db, Registry: NewRegistry(db), Backend: backend, Jobs: jm, DataDir: dataDir}, nil
}

func (s *Service) RestartPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.restartPending
}

func (s *Service) setRestartPending() {
	s.mu.Lock()
	s.restartPending = true
	s.mu.Unlock()
}

func (s *Service) List(ctx context.Context) ([]ManagedApp, error) {
	return s.Registry.List(ctx)
}

func (s *Service) Search(ctx context.Context, query string) ([]SearchResult, error) {
	if err := validateSearchQuery(query); err != nil {
		return nil, err
	}
	return s.Backend.Search(ctx, query)
}

func (s *Service) Reconcile(ctx context.Context) error {
	apps, err := s.Registry.List(ctx)
	if err != nil {
		return err
	}
	for _, a := range apps {
		if a.Source != SourceWinget || a.Status == StatusUninstalled {
			continue
		}
		installed, err := s.Backend.IsInstalled(ctx, a.WingetID)
		if err != nil {
			continue
		}
		if !installed {
			_ = s.Registry.UpdateStatus(ctx, a.ID, StatusMissing)
		}
	}
	return nil
}

func (s *Service) findSearchResult(ctx context.Context, id string) (SearchResult, bool) {
	res, err := s.Backend.Search(ctx, id)
	if err != nil {
		return SearchResult{}, false
	}
	for _, r := range res {
		if r.ID == id {
			return r, true
		}
	}
	return SearchResult{}, false
}

func (s *Service) InstallWinget(ctx context.Context, wingetID string) (string, error) {
	if err := validateWingetID(wingetID); err != nil {
		return "", err
	}
	s.installMu.Lock()
	defer s.installMu.Unlock()
	if !s.Backend.Available() {
		return "", ErrUnavailable
	}
	installed, err := s.Backend.IsInstalled(ctx, wingetID)
	if err != nil {
		// fail closed
		return "", fmt.Errorf("check existing installation: %w", err)
	}
	if installed {
		return "", ErrAlreadyInstalled
	}
	if s.Jobs == nil {
		return "", fmt.Errorf("jobs unavailable")
	}
	j, err := s.Jobs.Create(ctx, "software-install", 0)
	if err != nil {
		return "", err
	}
	sr, _ := s.findSearchResult(ctx, wingetID)
	displayName := sr.Name
	if displayName == "" {
		displayName = wingetID
	}
	version := sr.Version
	s.Jobs.Start(j, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		logw := &jobLogWriter{jobs: s.Jobs, id: j.ID, ctx: ctx}
		code, err := s.Backend.Install(ctx, wingetID, logw)
		if err != nil {
			_ = s.Registry.Add(ctx, ManagedApp{
				ID: wingetID, Source: SourceWinget, DisplayName: displayName,
				WingetID: wingetID, Version: version, InstalledAt: time.Now().UTC().Format(time.RFC3339),
				InstallJobID: j.ID, Status: StatusFailed,
			})
			return err
		}
		status := StatusInstalled
		if code == 3010 || code == 1641 {
			s.setRestartPending()
		}
		if code != 0 && code != 3010 && code != 1641 {
			_ = s.Registry.Add(ctx, ManagedApp{
				ID: wingetID, Source: SourceWinget, DisplayName: displayName,
				WingetID: wingetID, Version: version, InstalledAt: time.Now().UTC().Format(time.RFC3339),
				InstallJobID: j.ID, Status: StatusFailed,
			})
			return fmt.Errorf("winget install exited %d", code)
		}
		return s.Registry.Add(ctx, ManagedApp{
			ID: wingetID, Source: SourceWinget, DisplayName: displayName,
			WingetID: wingetID, Version: version, InstalledAt: time.Now().UTC().Format(time.RFC3339),
			InstallJobID: j.ID, Status: status,
		})
	})
	return j.ID, nil
}

func (s *Service) Updates(ctx context.Context) ([]Update, error) {
	apps, err := s.Registry.List(ctx)
	if err != nil {
		return nil, err
	}
	managedIDs := map[string]bool{}
	for _, a := range apps {
		if a.Source == SourceWinget && a.Status == StatusInstalled {
			managedIDs[a.WingetID] = true
		}
	}
	all, err := s.Backend.ListUpgrades(ctx)
	if err != nil {
		return nil, err
	}
	var result []Update
	for _, u := range all {
		if managedIDs[u.ID] {
			// attach installed version
			for _, a := range apps {
				if a.WingetID == u.ID {
					u.InstalledVersion = a.Version
				}
			}
			result = append(result, u)
		}
	}
	return result, nil
}

func (s *Service) Upgrade(ctx context.Context, managedID string) (string, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	app, err := s.Registry.Get(ctx, managedID)
	if err != nil {
		return "", err
	}
	if app.Source != SourceWinget {
		return "", ErrNotManaged
	}
	if app.Status != StatusInstalled {
		return "", fmt.Errorf("la aplicación no está instalada")
	}
	if err := validateWingetID(app.WingetID); err != nil {
		return "", err
	}
	if !s.Backend.Available() {
		return "", ErrUnavailable
	}
	if s.Jobs == nil {
		return "", fmt.Errorf("jobs unavailable")
	}
	j, err := s.Jobs.Create(ctx, "software-upgrade", 0)
	if err != nil {
		return "", err
	}
	s.Jobs.Start(j, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		logw := &jobLogWriter{jobs: s.Jobs, id: j.ID, ctx: ctx}
		code, err := s.Backend.Upgrade(ctx, app.WingetID, logw)
		if err != nil {
			return err
		}
		if code != 0 && code != 3010 && code != 1641 {
			return fmt.Errorf("winget upgrade exited %d", code)
		}
		if code == 3010 || code == 1641 {
			s.setRestartPending()
		}
		// update version from search? best effort
		if sr, ok := s.findSearchResult(ctx, app.WingetID); ok && sr.Version != "" {
			_ = s.Registry.UpdateVersion(ctx, managedID, sr.Version)
		}
		_ = s.Registry.UpdateStatus(ctx, managedID, StatusInstalled)
		return nil
	})
	return j.ID, nil
}

func (s *Service) Uninstall(ctx context.Context, managedID string) (string, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	app, err := s.Registry.Get(ctx, managedID)
	if err != nil {
		return "", err
	}
	if app.Source != SourceWinget {
		return "", ErrNotManaged
	}
	if app.Status != StatusInstalled {
		return "", fmt.Errorf("la aplicación no está instalada")
	}
	if err := validateWingetID(app.WingetID); err != nil {
		return "", err
	}
	if !s.Backend.Available() {
		return "", ErrUnavailable
	}
	if s.Jobs == nil {
		return "", fmt.Errorf("jobs unavailable")
	}
	j, err := s.Jobs.Create(ctx, "software-uninstall", 0)
	if err != nil {
		return "", err
	}
	s.Jobs.Start(j, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		logw := &jobLogWriter{jobs: s.Jobs, id: j.ID, ctx: ctx}
		code, err := s.Backend.Uninstall(ctx, app.WingetID, logw)
		if err != nil {
			return err
		}
		if code != 0 && code != 3010 && code != 1641 {
			return fmt.Errorf("winget uninstall exited %d", code)
		}
		if code == 3010 || code == 1641 {
			s.setRestartPending()
		}
		return s.Registry.UpdateStatus(ctx, managedID, StatusUninstalled)
	})
	return j.ID, nil
}

func (s *Service) Remove(ctx context.Context, managedID string) error {
	return s.Registry.Remove(ctx, managedID)
}

type jobLogWriter struct {
	jobs    *jobs.Manager
	id      string
	ctx     context.Context
	written int
}

func (w *jobLogWriter) Write(p []byte) (int, error) {
	if w.written < (1 << 20) {
		remaining := (1 << 20) - w.written
		chunk := p
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		if w.jobs != nil && len(chunk) > 0 {
			_, _ = w.jobs.Update(w.ctx, w.id, jobs.Running, -1, "en curso", string(chunk))
		}
		w.written += len(p)
	}
	return len(p), nil
}
