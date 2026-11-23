// SPDX-License-Identifier: Apache-2.0
package software

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
)

func setupSvc(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`CREATE TABLE managed_apps (id TEXT PRIMARY KEY, source TEXT NOT NULL, display_name TEXT NOT NULL, winget_id TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', installed_at TEXT NOT NULL, install_job_id TEXT NOT NULL DEFAULT '', uninstall_info TEXT NOT NULL DEFAULT '', status TEXT NOT NULL)`,
		`CREATE TABLE jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, state TEXT NOT NULL, total_bytes INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, message TEXT NOT NULL, log TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	jm, err := jobs.New(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(db, NewDisabledBackend("disabled"), jm, dir)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestUninstallUploadedHashMismatch(t *testing.T) {
	svc := setupSvc(t)
	ctx := context.Background()
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "u.exe")
	if err := os.WriteFile(exe, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := UninstallEntry{Key: "k", DisplayName: "App", QuietUninstallString: exe + " /S", UninstallerSHA256: "deadbeef"}
	b, _ := json.Marshal(entry)
	if err := svc.Registry.Add(ctx, ManagedApp{ID: "u1", Source: SourceUpload, DisplayName: "App", InstalledAt: "now", UninstallInfo: string(b), Status: StatusInstalled}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UninstallUploaded(ctx, "u1", true); err == nil || !strings.Contains(err.Error(), "manipulado") {
		t.Fatalf("expected hash mismatch error, got %v", err)
	}
}

func TestUninstallUploadedHKCUConfirm(t *testing.T) {
	svc := setupSvc(t)
	ctx := context.Background()
	entry := UninstallEntry{Key: "k", DisplayName: "App", Root: "HKCU", UninstallString: "C:\\Windows\\System32\\notepad.exe"}
	b, _ := json.Marshal(entry)
	if err := svc.Registry.Add(ctx, ManagedApp{ID: "u2", Source: SourceUpload, DisplayName: "App", InstalledAt: "now", UninstallInfo: string(b), Status: StatusInstalled}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UninstallUploaded(ctx, "u2", false); err == nil || !strings.Contains(err.Error(), "confirmación") {
		t.Fatalf("expected confirm error, got %v", err)
	}
}
