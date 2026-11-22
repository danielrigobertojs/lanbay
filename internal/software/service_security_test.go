// SPDX-License-Identifier: Apache-2.0
package software

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
)

func TestInstallUploadedRejectsOutsidePath(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "t.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS managed_apps (id TEXT PRIMARY KEY, source TEXT NOT NULL, display_name TEXT NOT NULL, winget_id TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', installed_at TEXT NOT NULL, install_job_id TEXT NOT NULL DEFAULT '', uninstall_info TEXT NOT NULL DEFAULT '', status TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, state TEXT NOT NULL, total_bytes INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, message TEXT NOT NULL, log TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	jm, err := jobs.New(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(db, NewDisabledBackend("disabled"), jm, dir)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "evil.exe")
	if err := os.WriteFile(outside, []byte("MZ"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = svc.InstallUploaded(context.Background(), InstallUploadOptions{Path: outside})
	if err == nil {
		t.Fatal("expected error for outside path")
	}
}
