// SPDX-License-Identifier: Apache-2.0
// Package software implements the managed-application registry and the
// winget backend behind software.Backend.
package software

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
)

var (
	ErrUnavailable      = errors.New("software module is unavailable on this platform")
	ErrAlreadyInstalled = errors.New("package is already installed outside Lanbay")
	ErrNotFound         = errors.New("managed application not found")
	ErrNotManaged       = errors.New("application is not managed by Lanbay")
	ErrDisabled         = errors.New("software module is disabled")
)

type Source string

const (
	SourceWinget Source = "winget"
	SourceUpload Source = "upload"
)

type Status string

const (
	StatusInstalled   Status = "installed"
	StatusFailed      Status = "failed"
	StatusUninstalled Status = "uninstalled"
	StatusMissing     Status = "missing"
)

type ManagedApp struct {
	ID            string `json:"id"`
	Source        Source `json:"source"`
	DisplayName   string `json:"display_name"`
	WingetID      string `json:"winget_id,omitempty"`
	Version       string `json:"version"`
	InstalledAt   string `json:"installed_at"`
	InstallJobID  string `json:"install_job_id"`
	UninstallInfo string `json:"uninstall_info,omitempty"`
	Status        Status `json:"status"`
}

type SearchResult struct {
	Name      string `json:"name"`
	ID        string `json:"id"`
	Version   string `json:"version"`
	Publisher string `json:"publisher,omitempty"`
	Match     string `json:"match,omitempty"`
	Source    string `json:"source,omitempty"`
}

// Type aliases for compatibility with alternative naming used by callers/tests.
type (
	Package            = SearchResult
	WingetPackage      = SearchResult
	WingetUpdate       = Update
	ManagedApplication = ManagedApp
	App                = ManagedApp
)

type Update struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	Source           string `json:"source,omitempty"`
}

type Backend interface {
	Available() bool
	Reason() string
	Search(ctx context.Context, query string) ([]SearchResult, error)
	IsInstalled(ctx context.Context, id string) (bool, error)
	Install(ctx context.Context, id string, out io.Writer) (int, error)
	Upgrade(ctx context.Context, id string, out io.Writer) (int, error)
	Uninstall(ctx context.Context, id string, out io.Writer) (int, error)
	ListUpgrades(ctx context.Context) ([]Update, error)
}

type disabledBackend struct{ reason string }

func (d *disabledBackend) Available() bool { return false }
func (d *disabledBackend) Reason() string  { return d.reason }
func (d *disabledBackend) Search(context.Context, string) ([]SearchResult, error) {
	return nil, ErrDisabled
}
func (d *disabledBackend) IsInstalled(context.Context, string) (bool, error) {
	return false, ErrDisabled
}
func (d *disabledBackend) Install(context.Context, string, io.Writer) (int, error) {
	return 0, ErrDisabled
}
func (d *disabledBackend) Upgrade(context.Context, string, io.Writer) (int, error) {
	return 0, ErrDisabled
}
func (d *disabledBackend) Uninstall(context.Context, string, io.Writer) (int, error) {
	return 0, ErrDisabled
}
func (d *disabledBackend) ListUpgrades(context.Context) ([]Update, error) {
	return nil, ErrDisabled
}

func NewBackend() Backend                      { return newBackend() }
func NewDisabledBackend(reason string) Backend { return &disabledBackend{reason: reason} }

type Registry struct {
	db *sql.DB
}

func NewRegistry(db *sql.DB) *Registry { return &Registry{db: db} }

func EnsureSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("nil database")
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS managed_apps (
		id TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		display_name TEXT NOT NULL,
		winget_id TEXT NOT NULL DEFAULT '',
		version TEXT NOT NULL DEFAULT '',
		installed_at TEXT NOT NULL,
		install_job_id TEXT NOT NULL DEFAULT '',
		uninstall_info TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL
	);`)
	if err != nil {
		return fmt.Errorf("migrate managed_apps: %w", err)
	}
	return nil
}

func (r *Registry) List(ctx context.Context) ([]ManagedApp, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,source,display_name,winget_id,version,installed_at,install_job_id,uninstall_info,status FROM managed_apps ORDER BY installed_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ManagedApp
	for rows.Next() {
		var a ManagedApp
		if err := rows.Scan(&a.ID, &a.Source, &a.DisplayName, &a.WingetID, &a.Version, &a.InstalledAt, &a.InstallJobID, &a.UninstallInfo, &a.Status); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (r *Registry) Get(ctx context.Context, id string) (ManagedApp, error) {
	var a ManagedApp
	err := r.db.QueryRowContext(ctx, "SELECT id,source,display_name,winget_id,version,installed_at,install_job_id,uninstall_info,status FROM managed_apps WHERE id=?", id).
		Scan(&a.ID, &a.Source, &a.DisplayName, &a.WingetID, &a.Version, &a.InstalledAt, &a.InstallJobID, &a.UninstallInfo, &a.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedApp{}, ErrNotFound
	}
	return a, err
}

func (r *Registry) Add(ctx context.Context, a ManagedApp) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO managed_apps(id,source,display_name,winget_id,version,installed_at,install_job_id,uninstall_info,status) VALUES(?,?,?,?,?,?,?,?,?)",
		a.ID, a.Source, a.DisplayName, a.WingetID, a.Version, a.InstalledAt, a.InstallJobID, a.UninstallInfo, a.Status)
	return err
}

func (r *Registry) UpdateStatus(ctx context.Context, id string, status Status) error {
	_, err := r.db.ExecContext(ctx, "UPDATE managed_apps SET status=? WHERE id=?", status, id)
	return err
}

func (r *Registry) UpdateVersion(ctx context.Context, id, version string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE managed_apps SET version=? WHERE id=?", version, id)
	return err
}

func (r *Registry) Remove(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM managed_apps WHERE id=?", id)
	return err
}
