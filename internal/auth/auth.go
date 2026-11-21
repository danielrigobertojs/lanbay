// SPDX-License-Identifier: Apache-2.0
// Package auth owns Lanbay's single-administrator password, durable sessions and login throttling.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"golang.org/x/crypto/argon2"
	"log/slog"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CookieName              = "__Host-lanbay_session"
	absoluteLifetime        = 7 * 24 * time.Hour
	argonTime        uint32 = 3
	argonMemory      uint32 = 64 * 1024
	argonThreads     uint8  = 4
	argonKeyLength   uint32 = 32
)

type Manager struct {
	db               *sql.DB
	idle             time.Duration
	now              func() time.Time
	mu               sync.Mutex
	attempts         map[string]*attempt
	global           *attempt
	passwordFailures map[string]*passwordAttempt
	hashSlots        chan struct{}
	loginQueue       chan struct{}
}
type attempt struct {
	failures           int
	last, blockedUntil time.Time
}

type passwordAttempt struct {
	failures int
	inFlight int
	revoked  bool
}

func New(db *sql.DB, idle time.Duration) *Manager {
	return &Manager{db: db, idle: idle, now: time.Now, attempts: map[string]*attempt{}, global: &attempt{}, passwordFailures: map[string]*passwordAttempt{}, hashSlots: make(chan struct{}, 2), loginQueue: make(chan struct{}, 16)}
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
	_, err = m.db.ExecContext(ctx, "INSERT OR REPLACE INTO settings(key,value) VALUES('password_change_required','true')")
	return password, err
}
func (m *Manager) SetPassword(ctx context.Context, password string) error {
	if !ValidPassword(password) {
		return errors.New("password must contain between 12 and 256 characters")
	}
	value, err := hash(password)
	if err != nil {
		return err
	}
	_, err = m.db.ExecContext(ctx, "INSERT OR REPLACE INTO settings(key,value) VALUES('password_hash',?),('password_change_required','false')", value)
	return err
}

// ChangePasswordForSession atomically changes the password only while the
// authenticated session still exists. It also completes the forced-change
// state and revokes every other session.
func (m *Manager) ChangePasswordForSession(ctx context.Context, id, password string) (bool, error) {
	if !ValidPassword(password) {
		return false, errors.New("password must contain between 12 and 256 characters")
	}
	value, err := hash(password)
	if err != nil {
		return false, err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin password change: %w", err)
	}
	defer tx.Rollback()
	key := digest(id)
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM sessions WHERE token_hash=?", key).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("check password-change session: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO settings(key,value) VALUES('password_hash',?),('password_change_required','false')", value); err != nil {
		return false, fmt.Errorf("store password hash: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE sessions SET change_required=FALSE WHERE token_hash=?", key); err != nil {
		return false, fmt.Errorf("complete password change: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash<>?", key); err != nil {
		return false, fmt.Errorf("revoke other sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit password change: %w", err)
	}
	m.mu.Lock()
	for other := range m.passwordFailures {
		if other != key {
			delete(m.passwordFailures, other)
		}
	}
	m.mu.Unlock()
	return true, nil
}
func ValidPassword(p string) bool { return len(p) >= 12 && len(p) <= 256 }

// VerifyPassword compares the configured secret without creating a session or consuming login rate-limit state.
func (m *Manager) VerifyPassword(ctx context.Context, password string) bool {
	if !ValidPassword(password) {
		return false
	}
	var encoded string
	if err := m.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key='password_hash'").Scan(&encoded); err != nil {
		return false
	}
	select {
	case m.hashSlots <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	ok, _ := verify(encoded, password)
	<-m.hashSlots
	return ok
}

// VerifyPasswordForSession limits password-change guesses independently from
// login throttling. It reserves one of five attempts before hashing, so a
// concurrent batch cannot queue past the limit. failed reports a completed,
// incorrect password check and is used for auditing; rejected reservations are
// deliberately not audited as hash attempts.
func (m *Manager) VerifyPasswordForSession(ctx context.Context, id, password string) (valid, failed bool, err error) {
	key := digest(id)
	m.mu.Lock()
	state := m.passwordFailures[key]
	if state == nil {
		state = &passwordAttempt{}
		m.passwordFailures[key] = state
	}
	if state.revoked || state.failures+state.inFlight >= 5 {
		m.mu.Unlock()
		return false, false, nil
	}
	state.inFlight++
	m.mu.Unlock()

	ok := m.VerifyPassword(ctx, password)
	m.mu.Lock()
	state.inFlight--
	if !ok {
		state.failures++
		failed = true
		if state.failures >= 5 {
			state.revoked = true
		}
	}
	revoke := state.revoked
	valid = ok && !revoke
	m.mu.Unlock()
	if revoke {
		// The revocation must survive a client that closes the connection
		// while the password hash is being computed: use a context detached
		// from the request with a short timeout so the DELETE reaches SQLite.
		revokeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		// Keep the revoked marker after deleting the durable session so every
		// request that already passed HTTP authentication still fails closed.
		if _, err := m.db.ExecContext(revokeCtx, "DELETE FROM sessions WHERE token_hash=?", key); err != nil {
			slog.Error("revoke password-change session", "error", err)
			return false, failed, fmt.Errorf("revoke password-change session: %w", err)
		}
	}
	return valid, failed, nil
}
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
	now, key := m.now(), attemptKey(ip)
	m.mu.Lock()
	m.cleanup(now)
	if now.Before(m.global.blockedUntil) || now.Before(m.get(key).blockedUntil) {
		m.mu.Unlock()
		return "", "", false, errors.New("too many attempts; try again later")
	}
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
	ok, rehash := verify(encoded, password)
	<-m.hashSlots
	m.mu.Lock()
	defer m.mu.Unlock()
	if !ok {
		m.fail(key, now)
		return "", "", false, errors.New("invalid credentials")
	}
	delete(m.attempts, key)
	if rehash {
		if replacement, e := hash(password); e == nil {
			_, _ = m.db.ExecContext(ctx, "UPDATE settings SET value=? WHERE key='password_hash'", replacement)
		}
	}
	id, e := random(32)
	if e != nil {
		return "", "", false, e
	}
	csrf, e := random(32)
	if e != nil {
		return "", "", false, e
	}
	_, e = m.db.ExecContext(ctx, "INSERT INTO sessions(token_hash,csrf_hash,created_at,last_seen,expires_at,change_required) VALUES(?,?,?,?,?,?)", digest(id), digest(csrf), now.Unix(), now.Unix(), now.Add(absoluteLifetime).Unix(), required == "true")
	if e != nil {
		return "", "", false, fmt.Errorf("create session: %w", e)
	}
	return id, csrf, required == "true", nil
}
func (m *Manager) Check(ctx context.Context, id, csrf string, write bool) (bool, bool) {
	m.mu.Lock()
	revoked := m.passwordFailures[digest(id)] != nil && m.passwordFailures[digest(id)].revoked
	m.mu.Unlock()
	if revoked {
		return false, false
	}
	now := m.now()
	var ch string
	var created, last, expires int64
	var required bool
	err := m.db.QueryRowContext(ctx, "SELECT csrf_hash,created_at,last_seen,expires_at,change_required FROM sessions WHERE token_hash=?", digest(id)).Scan(&ch, &created, &last, &expires, &required)
	if err != nil || now.Unix() >= expires || now.Sub(time.Unix(last, 0)) > m.idle || now.Sub(time.Unix(created, 0)) > absoluteLifetime {
		m.logout(ctx, id, true)
		return false, false
	}
	if write && (csrf == "" || subtle.ConstantTimeCompare([]byte(ch), []byte(digest(csrf))) != 1) {
		return false, false
	}
	_, _ = m.db.ExecContext(ctx, "UPDATE sessions SET last_seen=? WHERE token_hash=?", now.Unix(), digest(id))
	return true, required
}
func (m *Manager) Logout(ctx context.Context, id string) {
	m.logout(ctx, id, false)
}

func (m *Manager) logout(ctx context.Context, id string, preserveAttempt bool) {
	_, _ = m.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash=?", digest(id))
	if preserveAttempt {
		// Check can arrive after a concurrent revocation has removed the
		// durable row; retaining the marker prevents that path from reopening
		// the five-attempt budget.
		return
	}
	m.mu.Lock()
	key := digest(id)
	if state := m.passwordFailures[key]; state == nil || !state.revoked {
		delete(m.passwordFailures, key)
	}
	m.mu.Unlock()
}
func (m *Manager) CompletePasswordChange(ctx context.Context, id string) {
	_, _ = m.db.ExecContext(ctx, "UPDATE sessions SET change_required=FALSE WHERE token_hash=?", digest(id))
}
func (m *Manager) RevokeOtherSessions(ctx context.Context, id string) {
	_, _ = m.db.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash<>?", digest(id))
	m.mu.Lock()
	for key := range m.passwordFailures {
		if key != digest(id) {
			delete(m.passwordFailures, key)
		}
	}
	m.mu.Unlock()
}
func (m *Manager) get(key string) *attempt {
	if a := m.attempts[key]; a != nil {
		return a
	}
	a := &attempt{}
	m.attempts[key] = a
	return a
}
func (m *Manager) fail(key string, n time.Time) {
	a := m.get(key)
	a.failures++
	a.last = n
	m.global.failures++
	m.global.last = n
	if a.failures >= 5 {
		a.blockedUntil = n.Add(time.Minute << min(a.failures-5, 5))
	}
	if m.global.failures >= 100 {
		m.global.blockedUntil = n.Add(time.Minute)
	}
}
func (m *Manager) cleanup(n time.Time) {
	for k, a := range m.attempts {
		if n.Sub(a.last) > 24*time.Hour {
			delete(m.attempts, k)
		}
	}
	if n.Sub(m.global.last) > time.Minute {
		m.global.failures = 0
		m.global.blockedUntil = time.Time{}
	}
}
func attemptKey(ip netip.Addr) string {
	ip = ip.Unmap()
	if ip.Is6() {
		return netip.PrefixFrom(ip, 64).Masked().String()
	}
	return ip.String()
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func hash(p string) (string, error) {
	salt, e := randomBytes(16)
	if e != nil {
		return "", e
	}
	value := argon2.IDKey([]byte(p), salt, argonTime, argonMemory, argonThreads, argonKeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(value)), nil
}
func verify(v, p string) (bool, bool) {
	x := strings.Split(v, "$")
	if len(x) != 6 || x[1] != "argon2id" || x[2] != "v=19" {
		return false, false
	}
	params := map[string]uint64{}
	for _, field := range strings.Split(x[3], ",") {
		kv := strings.SplitN(field, "=", 2)
		if len(kv) != 2 {
			return false, false
		}
		n, e := strconv.ParseUint(kv[1], 10, 32)
		if e != nil {
			return false, false
		}
		params[kv[0]] = n
	}
	memory, cost, threads := uint32(params["m"]), uint32(params["t"]), uint8(params["p"])
	salt, e := base64.RawStdEncoding.DecodeString(x[4])
	if e != nil || memory == 0 || cost == 0 || threads == 0 {
		return false, false
	}
	want, e := base64.RawStdEncoding.DecodeString(x[5])
	if e != nil {
		return false, false
	}
	got := argon2.IDKey([]byte(p), salt, cost, memory, threads, uint32(len(want)))
	ok := subtle.ConstantTimeCompare(got, want) == 1
	return ok, ok && (memory != argonMemory || cost != argonTime || threads != argonThreads || len(want) != int(argonKeyLength))
}
func digest(s string) string {
	x := sha256.Sum256([]byte(s))
	return base64.RawStdEncoding.EncodeToString(x[:])
}
func random(n int) (string, error) {
	b, e := randomBytes(n)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func randomBytes(n int) ([]byte, error) { b := make([]byte, n); _, e := rand.Read(b); return b, e }
