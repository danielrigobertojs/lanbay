// SPDX-License-Identifier: Apache-2.0
package software

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
)

type pendingInstall struct {
	entries []UninstallEntry
	path    string
}

type Service struct {
	db       *sql.DB
	Registry *Registry
	Backend  Backend
	Jobs     *jobs.Manager
	DataDir  string

	mu             sync.Mutex
	restartPending bool
	installMu      sync.Mutex

	pendingMu      sync.Mutex
	pendingChoices map[string]pendingInstall
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

// DecideInstalled interprets winget list output per S4.
func DecideInstalled(code int, out string, id string) (bool, error) {
	const notInstalledCode = 0x8A150014
	if code == notInstalledCode {
		return false, nil
	}
	if code != 0 {
		return false, fmt.Errorf("winget list exited 0x%X", code)
	}
	res, err := ParseInstalledList(out)
	if err != nil {
		return false, err
	}
	for _, r := range res {
		if err := ValidateResultSource(r.Source); err != nil {
			return false, err
		}
		if r.ID == id {
			return true, nil
		}
	}
	return false, fmt.Errorf("winget list did not report an exact match")
}

func NewService(db *sql.DB, backend Backend, jm *jobs.Manager, dataDir string) (*Service, error) {
	if err := EnsureSchema(context.Background(), db); err != nil {
		return nil, err
	}
	svc := &Service{db: db, Registry: NewRegistry(db), Backend: backend, Jobs: jm, DataDir: dataDir, pendingChoices: map[string]pendingInstall{}}
	if err := EnsureStagingDir(svc.StagingDir()); err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	return svc, nil
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
	if app.Source == SourceUpload {
		return s.UninstallUploaded(ctx, managedID)
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

func (s *Service) StagingDir() string {
	dir := filepath.Join(s.DataDir, "staging")
	_ = EnsureStagingDir(dir)
	return dir
}

func (s *Service) InspectInstaller(path string) (InstallerInfo, error) {
	safe, err := ResolveStagingPath(s.StagingDir(), path)
	if err != nil {
		return InstallerInfo{}, err
	}
	return InspectInstaller(safe)
}

func (s *Service) PendingInstallChoices(jobID string) []UninstallEntry {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	out := make([]UninstallEntry, len(s.pendingChoices[jobID].entries))
	copy(out, s.pendingChoices[jobID].entries)
	return out
}

func (s *Service) ConfirmInstalled(ctx context.Context, jobID string, chosen []string, keep bool) error {
	s.pendingMu.Lock()
	pending := s.pendingChoices[jobID]
	choices := pending.entries
	delete(s.pendingChoices, jobID)
	s.pendingMu.Unlock()
	allowed := map[string]bool{}
	for _, c := range choices {
		allowed[c.Key] = true
	}
	var selected []UninstallEntry
	for _, k := range chosen {
		for _, c := range choices {
			if c.Key == k && allowed[k] {
				selected = append(selected, c)
			}
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("no se seleccionaron entradas nuevas")
	}
	// Register first selected entry as the managed app.
	e := selected[0]
	id := "upload:" + e.Key
	now := time.Now().UTC().Format(time.RFC3339)
	uninstallInfo := e.Key
	if e.ProductCode != "" {
		uninstallInfo = "msi:" + e.ProductCode
	}
	b, _ := json.Marshal(e)
	if len(b) > 0 {
		uninstallInfo = string(b)
	}
	if !keep && pending.path != "" {
		_ = RemoveStagingFile(pending.path)
	}
	return s.Registry.Add(ctx, ManagedApp{
		ID:            id,
		Source:        SourceUpload,
		DisplayName:   e.DisplayName,
		Version:       e.DisplayVersion,
		InstalledAt:   now,
		InstallJobID:  jobID,
		UninstallInfo: uninstallInfo,
		Status:        StatusInstalled,
	})
}

func (s *Service) InstallUploaded(ctx context.Context, opts InstallUploadOptions) (string, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	if !s.Backend.Available() && false {
		// upload installers do not require winget
	}
	if s.Jobs == nil {
		return "", fmt.Errorf("jobs unavailable")
	}
	if opts.Path == "" && opts.Name != "" {
		opts.Path = opts.Name
	}
	if opts.Path == "" && opts.InstallerID != "" {
		opts.Path = opts.InstallerID
	}
	if opts.Path == "" {
		return "", fmt.Errorf("path requerido")
	}
	safe, err := ResolveStagingPath(s.StagingDir(), opts.Path)
	if err != nil {
		return "", err
	}
	opts.Path = safe
	for _, a := range opts.Args {
		if len(a) > 1024 {
			return "", fmt.Errorf("argumento demasiado largo")
		}
		if strings.ContainsAny(a, "\"\x00\n\r") {
			return "", fmt.Errorf("carácter no permitido en argumento")
		}
	}
	if !StagingFileExists(opts.Path) {
		return "", fmt.Errorf("instalador no encontrado en staging")
	}
	j, err := s.Jobs.Create(ctx, "installer-install", 0)
	if err != nil {
		return "", err
	}
	s.Jobs.Start(j, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		logw := &jobLogWriter{jobs: s.Jobs, id: j.ID, ctx: ctx}
		jobTemp, _ := JobTempDir(s.DataDir, j.ID)
		defer RemoveAllPath(jobTemp)
		info, err := InspectInstaller(opts.Path)
		if err != nil {
			return err
		}
		if info.Authenticode.Signed && !info.Authenticode.Valid {
			return fmt.Errorf("firma Authenticode inválida o manipulada")
		}
		if info.Authenticode.Status != "Valid" && info.Authenticode.Status != "Unsigned" && !info.Authenticode.Signed {
			return fmt.Errorf("estado Authenticode desconocido o no verificable")
		}
		if !info.Authenticode.Signed {
			if !opts.ConfirmUnsigned {
				return fmt.Errorf("instalador sin firma: confirmación requerida")
			}
			if opts.ConfirmName == "" || opts.ConfirmName != filepath.Base(opts.Path) {
				return fmt.Errorf("confirmación de nombre incorrecta")
			}
		}
		before, _ := SnapshotUninstallKeys(ctx)
		ext := strings.ToLower(filepath.Ext(opts.Path))
		if ext == ".msi" {
			code, err := ReadMSIProductCode(opts.Path)
			if err == nil && code != "" {
				if !regexp.MustCompile(`^\{[0-9A-Fa-f]{8}-([0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}\}$`).MatchString(code) {
					return fmt.Errorf("ProductCode inválido")
				}
			}
		}
		var code int
		if ext == ".msi" {
			logPath := opts.Path + ".log"
			code, _, err = runCommand(ctx, MsiexecPath(), []string{"/i", opts.Path, "/qn", "/norestart", "/l*v", logPath}, installerEnv(jobTemp), jobTemp, logw)
		} else {
			args := []string{}
			if !opts.Interactive {
				if opts.Preset != "" {
					args = append(args, PresetArgs(opts.Preset)...)
				}
				args = append(args, opts.Args...)
			} else {
				args = append(args, opts.Args...)
			}
			code, _, err = runCommand(ctx, opts.Path, args, installerEnv(jobTemp), jobTemp, logw)
		}
		if err != nil {
			return err
		}
		if code != 0 && code != 3010 && code != 1641 {
			return fmt.Errorf("instalador salió con código %d", code)
		}
		if code == 3010 || code == 1641 {
			s.setRestartPending()
		}
		after, _ := SnapshotUninstallKeys(ctx)
		diff := DiffUninstallKeys(before, after)
		if len(diff) == 1 {
			e := diff[0]
			// Compute SHA-256 of the uninstaller executable (if parseable).
			for _, cand := range []string{e.QuietUninstallString, e.UninstallString} {
				parts := splitCommand(cand)
				if len(parts) > 0 {
					if h, err := SHA256File(parts[0]); err == nil {
						e.UninstallerSHA256 = h
						break
					}
				}
			}
			id := "upload:" + e.Key
			uninstallInfo := e.Key
			if e.ProductCode != "" {
				uninstallInfo = "msi:" + e.ProductCode
			}
			b, _ := json.Marshal(e)
			if len(b) > 0 {
				uninstallInfo = string(b)
			}
			_ = s.Registry.Add(ctx, ManagedApp{
				ID:            id,
				Source:        SourceUpload,
				DisplayName:   e.DisplayName,
				Version:       e.DisplayVersion,
				InstalledAt:   time.Now().UTC().Format(time.RFC3339),
				InstallJobID:  j.ID,
				UninstallInfo: uninstallInfo,
				Status:        StatusInstalled,
			})
		} else if len(diff) > 1 {
			s.pendingMu.Lock()
			s.pendingChoices[j.ID] = pendingInstall{entries: diff, path: opts.Path}
			s.pendingMu.Unlock()
			_, _ = s.Jobs.Update(ctx, j.ID, jobs.Completed, -1, "seleccione las entradas instaladas", "")
			// prevent Start from overwriting message? Start will set completed; okay
		} else {
			_, _ = s.Jobs.Update(ctx, j.ID, jobs.Completed, -1, "no rastreable", "")
		}
		if !opts.KeepInstaller && len(diff) <= 1 {
			_ = RemoveStagingFile(opts.Path)
		}
		return nil
	})
	return j.ID, nil
}

func (s *Service) UninstallUploaded(ctx context.Context, managedID string) (string, error) {
	s.installMu.Lock()
	defer s.installMu.Unlock()
	app, err := s.Registry.Get(ctx, managedID)
	if err != nil {
		return "", err
	}
	if app.Source != SourceUpload {
		return "", ErrNotManaged
	}
	if s.Jobs == nil {
		return "", fmt.Errorf("jobs unavailable")
	}
	j, err := s.Jobs.Create(ctx, "installer-uninstall", 0)
	if err != nil {
		return "", err
	}
	s.Jobs.Start(j, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		logw := &jobLogWriter{jobs: s.Jobs, id: j.ID, ctx: ctx}
		jobTemp, _ := JobTempDir(s.DataDir, j.ID)
		defer RemoveAllPath(jobTemp)
		// parse uninstall info
		var entry UninstallEntry
		_ = json.Unmarshal([]byte(app.UninstallInfo), &entry)
		if entry.Key == "" && app.UninstallInfo != "" {
			// could be msi: code or key
		}
		if strings.HasPrefix(app.UninstallInfo, "msi:") {
			code := strings.TrimPrefix(app.UninstallInfo, "msi:")
			if !regexp.MustCompile(`^\{[0-9A-Fa-f]{8}-([0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}\}$`).MatchString(code) {
				return fmt.Errorf("ProductCode inválido")
			}
			c, _, err := runCommand(ctx, MsiexecPath(), []string{"/x", code, "/qn", "/norestart"}, installerEnv(jobTemp), jobTemp, logw)
			if err != nil {
				return err
			}
			if c != 0 && c != 3010 && c != 1641 {
				return fmt.Errorf("msiexec uninstall exited %d", c)
			}
			if c == 3010 || c == 1641 {
				s.setRestartPending()
			}
		} else if entry.QuietUninstallString != "" {
			if entry.UninstallerSHA256 == "" {
				return fmt.Errorf("sin hash de desinstalador: rechazado")
			}
			parts := splitCommand(entry.QuietUninstallString)
			if len(parts) == 0 {
				return fmt.Errorf("quiet uninstall string vacío")
			}
			exe := parts[0]
			args := parts[1:]
			if entry.UninstallerSHA256 != "" {
				h, err := SHA256File(exe)
				if err != nil || h != entry.UninstallerSHA256 {
					return fmt.Errorf("desinstalador manipulado o no verificable")
				}
			}
			c, _, err := runCommand(ctx, exe, args, installerEnv(jobTemp), jobTemp, logw)
			if err != nil {
				return err
			}
			if c != 0 && c != 3010 && c != 1641 {
				return fmt.Errorf("uninstall exited %d", c)
			}
		} else if entry.UninstallString != "" {
			if entry.UninstallerSHA256 == "" {
				return fmt.Errorf("sin hash de desinstalador: rechazado")
			}
			parts := splitCommand(entry.UninstallString)
			if len(parts) == 0 {
				return fmt.Errorf("uninstall string vacío")
			}
			exe := parts[0]
			args := parts[1:]
			if entry.UninstallerSHA256 != "" {
				if h, err := SHA256File(exe); err == nil && h != entry.UninstallerSHA256 {
					return fmt.Errorf("desinstalador manipulado")
				}
			}
			c, _, err := runCommand(ctx, exe, args, installerEnv(jobTemp), jobTemp, logw)
			if err != nil {
				return err
			}
			if c != 0 && c != 3010 && c != 1641 {
				return fmt.Errorf("uninstall exited %d", c)
			}
		} else if strings.HasPrefix(app.UninstallInfo, "{") {
			code := app.UninstallInfo
			c, _, err := runCommand(ctx, MsiexecPath(), []string{"/x", code, "/qn", "/norestart"}, installerEnv(jobTemp), jobTemp, logw)
			if err != nil {
				return err
			}
			if c != 0 && c != 3010 && c != 1641 {
				return fmt.Errorf("msiexec uninstall exited %d", c)
			}
		} else {
			return fmt.Errorf("sin información de desinstalación")
		}
		return s.Registry.UpdateStatus(ctx, managedID, StatusUninstalled)
	})
	return j.ID, nil
}

// splitCommand splits a command line into argv, honouring double-quoted
// segments with spaces (e.g. "C:\Program Files\x\uninstall.exe" /S).
func splitCommand(line string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '"':
			inQuote = !inQuote
		case (c == ' ' || c == '\t') && !inQuote:
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func (s *Service) StageInstaller(data []byte, originalName string) (string, error) {
	return StageUploadedFile(s.StagingDir(), originalName, data)
}
