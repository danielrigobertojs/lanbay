// SPDX-License-Identifier: Apache-2.0
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenSetsBusyTimeout(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "lanbay.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	if timeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", timeout)
	}
}
