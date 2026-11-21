// SPDX-License-Identifier: Apache-2.0
// Package store is the SQLite boundary for Lanbay state.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open creates a SQLite connection using the pure-Go driver required by Lanbay.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS audit_log (id INTEGER PRIMARY KEY, created_at TEXT NOT NULL, ip TEXT NOT NULL, action TEXT NOT NULL, result TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (token_hash TEXT PRIMARY KEY, csrf_hash TEXT NOT NULL, created_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, expires_at INTEGER NOT NULL, change_required BOOLEAN NOT NULL);
CREATE TABLE IF NOT EXISTS jobs (id TEXT PRIMARY KEY, type TEXT NOT NULL, state TEXT NOT NULL, total_bytes INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, message TEXT NOT NULL, log TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS uploads (id TEXT PRIMARY KEY, job_id TEXT NOT NULL, destination TEXT NOT NULL, temporary TEXT NOT NULL, size_bytes INTEGER NOT NULL, offset_bytes INTEGER NOT NULL, state TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS managed_apps (id TEXT PRIMARY KEY, source TEXT NOT NULL, display_name TEXT NOT NULL, winget_id TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '', installed_at TEXT NOT NULL, install_job_id TEXT NOT NULL DEFAULT '', uninstall_info TEXT NOT NULL DEFAULT '', status TEXT NOT NULL);`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return db, nil
}
