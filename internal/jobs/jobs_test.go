// SPDX-License-Identifier: Apache-2.0
package jobs

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/danielrigobertojs/lanbay/internal/store"
)

func TestRestartMarksActiveJobsInterrupted(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	j, err := m.Create(context.Background(), "upload", 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Update(context.Background(), j.ID, Running, 2, "en curso", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := New(db); err != nil {
		t.Fatal(err)
	}
	got, err := m.Get(context.Background(), j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Interrupted {
		t.Fatalf("state=%s", got.State)
	}
}

func TestSubscribeReceivesProgress(t *testing.T) {
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "jobs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m, _ := New(db)
	events, stop := m.Subscribe()
	defer stop()
	j, _ := m.Create(context.Background(), "upload", 9)
	if _, err := m.Update(context.Background(), j.ID, Running, 4, "en curso", ""); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		<-events
	}
}
