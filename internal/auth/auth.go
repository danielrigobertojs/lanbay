// SPDX-License-Identifier: Apache-2.0
// Package auth owns Lanbay's single-administrator password, sessions and login throttling.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/argon2"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const CookieName = "lanbay_session"

type Manager struct {
	db         *sql.DB
	idle       time.Duration
	now        func() time.Time
	mu         sync.Mutex
	sessions   map[string]session
	attempts   map[netip.Addr]*attempt
	hashSlots  chan struct{}
	loginQueue chan struct{}
}
type session struct {
	csrf           string
	lastSeen       time.Time
	changeRequired bool
}
type attempt struct {
	failures, attempts int
	last, blockedUntil time.Time
}

func New(db *sql.DB, idle time.Duration) *Manager {
	return &Manager{db: db, idle: idle, now: time.Now, sessions: map[string]session{}, attempts: map[netip.Addr]*attempt{}, hashSlots: make(chan struct{}, 2), loginQueue: make(chan struct{}, 16)}
}
func (m *Manager) EnsurePassword(ctx context.Context) (string, error) {
	var count int
	if err := m.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM settings WHERE key='password_hash'").Scan(&count); err != nil {
		return "", err
	}
	if count > 0 {
		return "", nil
	}
	password, err := random(24)
	if err != nil {
		return "", err
	}
	if err = m.SetPassword(ctx, password); err != nil {
		return "", err
	}
	if _, err = m.db.ExecContext(ctx, "INSERT OR REPLACE INTO settings(key,value) VALUES('password_change_required','true')"); err != nil {
		return "", err
	}
	return password, nil
}
func (m *Manager) SetPassword(ctx context.Context, password string) error {
	if !ValidPassword(password) {
		return errors.New("password must contain between 12 and 256 characters")
	}
	salt, err := randomBytes(16)
	if err != nil {
		return err
	}
	hash := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, 32)
	encoded := base64.RawStdEncoding.EncodeToString(salt) + ":" + base64.RawStdEncoding.EncodeToString(hash)
	_, err = m.db.ExecContext(ctx, "INSERT OR REPLACE INTO settings(key,value) VALUES('password_hash',?),('password_change_required','false')", encoded)
	return err
}
func ValidPassword(password string) bool { return len(password) >= 12 && len(password) <= 256 }
func (m *Manager) Login(ctx context.Context, ip netip.Addr, password string) (string, string, bool, error) {
	if !ValidPassword(password) {
		return "", "", false, errors.New("invalid credentials")
	}
	select {
	case m.loginQueue <- struct{}{}:
		defer func() { <-m.loginQueue }()
	default:
		return "", "", false, errors.New("too many login requests")
	}
	m.mu.Lock()
	now := m.now()
	a := m.getAttempt(ip, now)
	if now.Before(a.blockedUntil) {
		m.mu.Unlock()
		return "", "", false, errors.New("too many attempts; try again later")
	}
	a.attempts++
	a.last = now
	m.mu.Unlock()
	var encoded, required string
	if err := m.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='password_hash'").Scan(&encoded); err != nil {
		return "", "", false, fmt.Errorf("load password hash: %w", err)
	}
	_ = m.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='password_change_required'").Scan(&required)
	select {
	case m.hashSlots <- struct{}{}:
	case <-ctx.Done():
		return "", "", false, ctx.Err()
	}
	ok := verify(encoded, password)
	<-m.hashSlots
	m.mu.Lock()
	defer m.mu.Unlock()
	a = m.getAttempt(ip, now)
	if !ok {
		a.failures++
		if a.failures >= 5 {
			delay := time.Minute << min(a.failures-5, 5)
			a.blockedUntil = now.Add(delay)
		}
		return "", "", false, errors.New("invalid credentials")
	}
	delete(m.attempts, ip)
	id, e := random(32)
	if e != nil {
		return "", "", false, e
	}
	csrf, e := random(32)
	if e != nil {
		return "", "", false, e
	}
	m.sessions[id] = session{csrf: csrf, lastSeen: now, changeRequired: required == "true"}
	return id, csrf, required == "true", nil
}
func (m *Manager) getAttempt(ip netip.Addr, now time.Time) *attempt {
	a := m.attempts[ip]
	if a == nil {
		a = &attempt{last: now}
		m.attempts[ip] = a
	}
	return a
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
func (m *Manager) Check(id, csrf string, mutating bool) (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok || m.now().Sub(s.lastSeen) > m.idle {
		delete(m.sessions, id)
		return false, false
	}
	if mutating && (csrf == "" || subtle.ConstantTimeCompare([]byte(s.csrf), []byte(csrf)) != 1) {
		return false, false
	}
	s.lastSeen = m.now()
	m.sessions[id] = s
	return true, s.changeRequired
}
func (m *Manager) Logout(id string) { m.mu.Lock(); defer m.mu.Unlock(); delete(m.sessions, id) }

// CompletePasswordChange lets the current session proceed after changing its
// first-run password.
func (m *Manager) CompletePasswordChange(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return
	}
	s.changeRequired = false
	m.sessions[id] = s
}

// RevokeOtherSessions invalidates every session except the one that changed the password.
func (m *Manager) RevokeOtherSessions(keep string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.sessions[keep]
	m.sessions = map[string]session{}
	if ok {
		m.sessions[keep] = current
	}
}

func verify(encoded, password string) bool {
	parts := strings.Split(encoded, ":")
	if len(parts) != 2 {
		return false
	}
	s, e := base64.RawStdEncoding.DecodeString(parts[0])
	if e != nil {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(parts[1])
	if e != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), s, 3, 64*1024, 4, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func random(n int) (string, error) {
	b, e := randomBytes(n)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func randomBytes(n int) ([]byte, error) { b := make([]byte, n); _, e := rand.Read(b); return b, e }
