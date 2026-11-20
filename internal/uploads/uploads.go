// SPDX-License-Identifier: Apache-2.0
// Package uploads implements bounded-memory resumable file uploads.
package uploads

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/danielrigobertojs/lanbay/internal/jobs"
	"github.com/danielrigobertojs/lanbay/internal/pathguard"
)

const (
	MinChunk = 8 << 20
	MaxChunk = 64 << 20
)

var (
	ErrOffset   = errors.New("upload offset does not match")
	ErrChunk    = errors.New("invalid upload chunk")
	ErrSpace    = errors.New("insufficient free disk space")
	ErrNotFound = errors.New("upload not found")
	idPattern   = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

type Session struct {
	ID          string `json:"id"`
	JobID       string `json:"job_id"`
	Destination string `json:"destination"`
	Size        int64  `json:"size_bytes"`
	Offset      int64  `json:"offset_bytes"`
	State       string `json:"state"`
	ChunkSize   int64  `json:"chunk_size"`
}
type Manager struct {
	db   *sql.DB
	jobs *jobs.Manager
}

func New(db *sql.DB, jm *jobs.Manager) *Manager { return &Manager{db: db, jobs: jm} }

func (m *Manager) Create(ctx context.Context, guard pathguard.Guard, parent, name string, size, chunk int64) (Session, error) {
	if size < 0 || chunk < MinChunk || chunk > MaxChunk {
		return Session{}, ErrChunk
	}
	destination, err := guard.CheckNew(parent, name, true)
	if err != nil {
		return Session{}, fmt.Errorf("authorize upload destination: %w", err)
	}
	if err := ensureSpace(parent, size); err != nil {
		return Session{}, err
	}
	j, err := m.jobs.Create(ctx, "upload", size)
	if err != nil {
		return Session{}, err
	}
	id := randomID()
	temporary := filepath.Join(filepath.Dir(destination), ".lanbay-upload-"+id+".part")
	// The generated temporary name is not client-controlled; it lives beside the final, guarded destination.
	created := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = m.db.ExecContext(ctx, "INSERT INTO uploads(id,job_id,destination,temporary,size_bytes,offset_bytes,state,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", id, j.ID, destination, temporary, size, 0, "active", created, created)
	if err != nil {
		return Session{}, fmt.Errorf("persist upload: %w", err)
	}
	_, _ = m.jobs.Update(ctx, j.ID, jobs.Running, 0, "esperando datos", "")
	return Session{ID: id, JobID: j.ID, Destination: destination, Size: size, State: "active", ChunkSize: chunk}, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Session, string, error) {
	if !idPattern.MatchString(id) {
		return Session{}, "", ErrNotFound
	}
	var s Session
	var temporary string
	err := m.db.QueryRowContext(ctx, "SELECT id,job_id,destination,temporary,size_bytes,offset_bytes,state FROM uploads WHERE id=?", id).Scan(&s.ID, &s.JobID, &s.Destination, &temporary, &s.Size, &s.Offset, &s.State)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, "", ErrNotFound
	}
	return s, temporary, err
}

func (m *Manager) Write(ctx context.Context, id string, offset, chunkSize int64, body io.Reader) (Session, error) {
	s, temporary, err := m.Get(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if s.State != "active" || offset != s.Offset {
		return Session{}, ErrOffset
	}
	if chunkSize <= 0 || chunkSize > MaxChunk || chunkSize > s.Size-s.Offset || (chunkSize < MinChunk && offset+chunkSize != s.Size) {
		return Session{}, ErrChunk
	}
	if err := ctx.Err(); err != nil {
		return Session{}, err
	}
	f, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return Session{}, fmt.Errorf("open temporary upload: %w", err)
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()
		return Session{}, err
	}
	written, copyErr := io.CopyN(f, body, chunkSize)
	closeErr := f.Close()
	if copyErr != nil {
		return Session{}, fmt.Errorf("stream chunk after %d bytes: %w", written, copyErr)
	}
	if closeErr != nil {
		return Session{}, closeErr
	}
	if written != chunkSize {
		return Session{}, ErrChunk
	}
	s.Offset += written
	state := "active"
	if s.Offset == s.Size {
		if err := os.Rename(temporary, s.Destination); err != nil {
			return Session{}, fmt.Errorf("atomically finalize upload: %w", err)
		}
		state = "completed"
	}
	s.State = state
	at := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = m.db.ExecContext(ctx, "UPDATE uploads SET offset_bytes=?,state=?,updated_at=? WHERE id=?", s.Offset, state, at, id); err != nil {
		return Session{}, err
	}
	jobState := jobs.Running
	message := "recibiendo"
	if state == "completed" {
		jobState = jobs.Completed
		message = "completado"
	}
	_, _ = m.jobs.Update(ctx, s.JobID, jobState, s.Offset, message, "")
	return s, nil
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	s, temporary, err := m.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := os.Remove(temporary); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove partial upload: %w", err)
	}
	if _, err = m.db.ExecContext(ctx, "UPDATE uploads SET state=?,updated_at=? WHERE id=?", "canceled", time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
		return err
	}
	_, err = m.jobs.Update(ctx, s.JobID, jobs.Canceled, s.Offset, "cancelado", "")
	return err
}
func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
