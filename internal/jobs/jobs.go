// SPDX-License-Identifier: Apache-2.0
// Package jobs persists asynchronous work and broadcasts its state to clients.
package jobs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type State string

const (
	Queued      State = "queued"
	Running     State = "running"
	Completed   State = "completed"
	Failed      State = "failed"
	Canceled    State = "canceled"
	Interrupted State = "interrupted"
)

type Job struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	State     State  `json:"state"`
	Total     int64  `json:"total_bytes"`
	Completed int64  `json:"completed_bytes"`
	Message   string `json:"message"`
	Log       string `json:"log"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Manager struct {
	db          *sql.DB
	mu          sync.Mutex
	subscribers map[chan Job]struct{}
	cancels     map[string]context.CancelFunc
}

func New(db *sql.DB) (*Manager, error) {
	if _, err := db.Exec("UPDATE jobs SET state=?, message=?, updated_at=? WHERE state IN (?,?)", Interrupted, "interrumpido por reinicio", now(), Queued, Running); err != nil {
		return nil, fmt.Errorf("mark interrupted jobs: %w", err)
	}
	return &Manager{db: db, subscribers: make(map[chan Job]struct{}), cancels: make(map[string]context.CancelFunc)}, nil
}

func (m *Manager) Create(ctx context.Context, kind string, total int64) (Job, error) {
	if kind == "" || total < 0 {
		return Job{}, fmt.Errorf("invalid job")
	}
	j := Job{ID: newID(), Type: kind, State: Queued, Total: total, Message: "en cola", CreatedAt: now(), UpdatedAt: now()}
	_, err := m.db.ExecContext(ctx, "INSERT INTO jobs(id,type,state,total_bytes,completed_bytes,message,log,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)", j.ID, j.Type, j.State, j.Total, 0, j.Message, "", j.CreatedAt, j.UpdatedAt)
	if err != nil {
		return Job{}, fmt.Errorf("create job: %w", err)
	}
	m.publish(j)
	return j, nil
}

func (m *Manager) Get(ctx context.Context, id string) (Job, error) {
	var j Job
	err := m.db.QueryRowContext(ctx, "SELECT id,type,state,total_bytes,completed_bytes,message,log,created_at,updated_at FROM jobs WHERE id=?", id).Scan(&j.ID, &j.Type, &j.State, &j.Total, &j.Completed, &j.Message, &j.Log, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	return j, nil
}

func (m *Manager) List(ctx context.Context) ([]Job, error) {
	rows, err := m.db.QueryContext(ctx, "SELECT id,type,state,total_bytes,completed_bytes,message,log,created_at,updated_at FROM jobs ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Type, &j.State, &j.Total, &j.Completed, &j.Message, &j.Log, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, j)
	}
	return result, rows.Err()
}

func (m *Manager) Update(ctx context.Context, id string, state State, completed int64, message, log string) (Job, error) {
	j, err := m.Get(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if completed >= 0 {
		j.Completed = completed
	}
	if state != "" {
		j.State = state
	}
	if message != "" {
		j.Message = message
	}
	if log != "" {
		j.Log += log
	}
	j.UpdatedAt = now()
	_, err = m.db.ExecContext(ctx, "UPDATE jobs SET state=?,completed_bytes=?,message=?,log=?,updated_at=? WHERE id=?", j.State, j.Completed, j.Message, j.Log, j.UpdatedAt, id)
	if err != nil {
		return Job{}, fmt.Errorf("update job: %w", err)
	}
	m.publish(j)
	return j, nil
}

func (m *Manager) Start(j Job, work func(context.Context) error) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancels[j.ID] = cancel
	m.mu.Unlock()
	go func() {
		_, _ = m.Update(context.Background(), j.ID, Running, -1, "en curso", "")
		err := work(ctx)
		m.mu.Lock()
		delete(m.cancels, j.ID)
		m.mu.Unlock()
		if err == nil {
			_, _ = m.Update(context.Background(), j.ID, Completed, -1, "completado", "")
		} else if ctx.Err() != nil {
			_, _ = m.Update(context.Background(), j.ID, Canceled, -1, "cancelado", "")
		} else {
			_, _ = m.Update(context.Background(), j.ID, Failed, -1, "falló", err.Error()+"\n")
		}
	}()
}

func (m *Manager) Cancel(ctx context.Context, id string) error {
	m.mu.Lock()
	cancel := m.cancels[id]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	_, err := m.Update(ctx, id, Canceled, -1, "cancelado", "")
	return err
}

func (m *Manager) Subscribe() (<-chan Job, func()) {
	c := make(chan Job, 16)
	m.mu.Lock()
	m.subscribers[c] = struct{}{}
	m.mu.Unlock()
	return c, func() {
		m.mu.Lock()
		if _, ok := m.subscribers[c]; ok {
			delete(m.subscribers, c)
			close(c)
		}
		m.mu.Unlock()
	}
}
func (m *Manager) publish(j Job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for c := range m.subscribers {
		select {
		case c <- j:
		default:
		}
	}
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
