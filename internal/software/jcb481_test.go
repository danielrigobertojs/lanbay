// SPDX-License-Identifier: Apache-2.0
package software

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
)

func jcb481SetupSvc(t *testing.T) *Service {
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

func TestJCB481MsiExecXUsesSystem32(t *testing.T) {
	svc := jcb481SetupSvc(t)
	ctx := context.Background()
	guid := "{12345678-1234-1234-1234-123456789012}"
	entry := UninstallEntry{Key: "k", DisplayName: "App", UninstallString: "MsiExec.exe /X" + guid}
	b, _ := json.Marshal(entry)
	if err := svc.Registry.Add(ctx, ManagedApp{ID: "u1", Source: SourceUpload, DisplayName: "App", InstalledAt: "now", UninstallInfo: string(b), Status: StatusInstalled}); err != nil {
		t.Fatal(err)
	}

	old := runCommand
	defer func() { runCommand = old }()
	var calledPath string
	var calledArgs []string
	runCommand = func(ctx context.Context, path string, args []string, env []string, dir string, out io.Writer) (int, []byte, error) {
		calledPath = path
		calledArgs = append([]string(nil), args...)
		return 0, nil, nil
	}

	jobID, err := svc.UninstallUploaded(ctx, "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	// wait for completion
	for i := 0; i < 100; i++ {
		j, err := svc.Jobs.Get(ctx, jobID)
		if err == nil && (j.State == jobs.Completed || j.State == jobs.Failed || j.State == jobs.Canceled) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := svc.Jobs.Get(ctx, jobID)
	if j.State != jobs.Completed {
		t.Fatalf("job state=%s (msg=%s)", j.State, j.Message)
	}
	if calledPath != MsiexecPath() {
		t.Fatalf("runCommand path=%q want %q", calledPath, MsiexecPath())
	}
	if len(calledArgs) < 2 || calledArgs[0] != "/x" || calledArgs[1] != guid {
		t.Fatalf("runCommand args=%v", calledArgs)
	}
}

func TestJCB481HKCURequiresConfirm(t *testing.T) {
	svc := jcb481SetupSvc(t)
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

func TestJCB481HashChangedRejected(t *testing.T) {
	svc := jcb481SetupSvc(t)
	ctx := context.Background()
	tmp := t.TempDir()
	exe := filepath.Join(tmp, "u.exe")
	if err := os.WriteFile(exe, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := UninstallEntry{Key: "k", DisplayName: "App", QuietUninstallString: exe + " /S", UninstallerSHA256: "deadbeef"}
	b, _ := json.Marshal(entry)
	if err := svc.Registry.Add(ctx, ManagedApp{ID: "u3", Source: SourceUpload, DisplayName: "App", InstalledAt: "now", UninstallInfo: string(b), Status: StatusInstalled}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UninstallUploaded(ctx, "u3", false); err == nil || !strings.Contains(err.Error(), "manipulado") {
		t.Fatalf("expected hash mismatch error, got %v", err)
	}
}
