// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"context"
	"database/sql"
	"encoding/base64"
	"golang.org/x/crypto/argon2"
	_ "modernc.org/sqlite"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

func manager(t *testing.T) *Manager {
	t.Helper()
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	db.SetMaxOpenConns(1)
	if _, e = db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY,value TEXT NOT NULL); CREATE TABLE sessions (token_hash TEXT PRIMARY KEY, csrf_hash TEXT NOT NULL, created_at INTEGER NOT NULL, last_seen INTEGER NOT NULL, expires_at INTEGER NOT NULL, change_required BOOLEAN NOT NULL)"); e != nil {
		t.Fatal(e)
	}
	return New(db, time.Hour)
}
func TestPasswordLoginAndCSRF(t *testing.T) {
	m := manager(t)
	if e := m.SetPassword(context.Background(), "correct horse battery"); e != nil {
		t.Fatal(e)
	}
	id, csrf, _, e := m.Login(context.Background(), netip.MustParseAddr("192.168.1.3"), "correct horse battery")
	if e != nil {
		t.Fatal(e)
	}
	if ok, _ := m.Check(context.Background(), id, "", true); ok {
		t.Fatal("missing CSRF accepted")
	}
	if ok, _ := m.Check(context.Background(), id, csrf, true); !ok {
		t.Fatal("valid session rejected")
	}
}
func TestRateLimit(t *testing.T) {
	m := manager(t)
	_ = m.SetPassword(context.Background(), "correct horse battery")
	ip := netip.MustParseAddr("10.0.0.4")
	for i := 0; i < 5; i++ {
		_, _, _, _ = m.Login(context.Background(), ip, "wrong password value")
	}
	if _, _, _, e := m.Login(context.Background(), ip, "correct horse battery"); e == nil {
		t.Fatal("rate limit missing")
	}
}

func TestLoginRejectsOversizedPasswordBeforeHash(t *testing.T) {
	m := manager(t)
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.9"), strings.Repeat("a", 257)); err == nil {
		t.Fatal("oversized password accepted")
	}
}

func TestCompletePasswordChange(t *testing.T) {
	m := manager(t)
	initial, err := m.EnsurePassword(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id, csrf, required, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.5"), initial)
	if err != nil || !required {
		t.Fatalf("first login = %q, %t, %v", id, required, err)
	}
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	m.CompletePasswordChange(context.Background(), id)
	if ok, stillRequired := m.Check(context.Background(), id, csrf, false); !ok || stillRequired {
		t.Fatalf("session after password change = %t, %t", ok, stillRequired)
	}
}

func TestVerifyPasswordDoesNotCreateSession(t *testing.T) {
	m := manager(t)
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	if !m.VerifyPassword(context.Background(), "correct horse battery") || m.VerifyPassword(context.Background(), "wrong password value") {
		t.Fatal("password verification mismatch")
	}
	var sessions int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions=%d err=%v", sessions, err)
	}
}

func TestVerifyPasswordForSessionRevokesAfterFiveFailures(t *testing.T) {
	m := manager(t)
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id, csrf, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.8"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if ok, _, err := m.VerifyPasswordForSession(context.Background(), id, "wrong password value"); err != nil || ok {
			t.Fatal("wrong password accepted")
		}
		if ok, _ := m.Check(context.Background(), id, csrf, false); !ok {
			t.Fatalf("session revoked after %d failures", i+1)
		}
	}
	if ok, _, err := m.VerifyPasswordForSession(context.Background(), id, "wrong password value"); err != nil || ok {
		t.Fatal("wrong password accepted")
	}
	if ok, _ := m.Check(context.Background(), id, csrf, false); ok {
		t.Fatal("session remains valid after five failures")
	}
}

func TestVerifyPasswordForSessionReservesConcurrentAttempts(t *testing.T) {
	m := manager(t)
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id, csrf, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.8"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	const requests = 20
	start := make(chan struct{})
	results := make(chan bool, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, failed, err := m.VerifyPasswordForSession(context.Background(), id, "wrong password value")
			if err != nil {
				t.Errorf("verify password: %v", err)
			}
			results <- failed
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	failed := 0
	for result := range results {
		if result {
			failed++
		}
	}
	if failed != 5 {
		t.Fatalf("hashed failures = %d, want 5", failed)
	}
	if ok, _ := m.Check(context.Background(), id, csrf, false); ok {
		t.Fatal("session remains valid after concurrent failures")
	}
	if ok, failed, err := m.VerifyPasswordForSession(context.Background(), id, "correct horse battery"); err != nil || ok || failed {
		t.Fatalf("correct password after revocation = %t, failed = %t", ok, failed)
	}
}

func TestChangePasswordForSessionRequiresExistingSession(t *testing.T) {
	m := manager(t)
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id, _, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.8"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	m.Logout(context.Background(), id)
	changed, err := m.ChangePasswordForSession(context.Background(), id, "new correct battery")
	if err != nil || changed {
		t.Fatalf("changed = %t, err = %v", changed, err)
	}
	if !m.VerifyPassword(context.Background(), "correct horse battery") || m.VerifyPassword(context.Background(), "new correct battery") {
		t.Fatal("password changed after session revocation")
	}
}

func TestVerifyPasswordForSessionRevokesWithCanceledContext(t *testing.T) {
	m := manager(t)
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id, _, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.8"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 5; i++ {
		if _, _, err := m.VerifyPasswordForSession(ctx, id, "wrong password value"); err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
	var sessions int
	if err := m.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions); err != nil || sessions != 0 {
		t.Fatalf("sessions=%d err=%v", sessions, err)
	}
}

func TestIPv6AttemptsShare64AndCleanup(t *testing.T) {
	m := manager(t)
	if attemptKey(netip.MustParseAddr("fd00::1")) != attemptKey(netip.MustParseAddr("fd00::2")) {
		t.Fatal("/64 grouping missing")
	}
	m.attempts["old"] = &attempt{last: time.Now().Add(-25 * time.Hour)}
	m.cleanup(time.Now())
	if _, ok := m.attempts["old"]; ok {
		t.Fatal("stale attempt not removed")
	}
}

func TestGlobalFailureBudget(t *testing.T) {
	m := manager(t)
	now := time.Now()
	for i := 0; i < 100; i++ {
		m.fail("source"+string(rune(i)), now)
	}
	if !now.Before(m.global.blockedUntil) {
		t.Fatal("global failure budget did not block")
	}
}

func TestLoginRehashesLegacyPHCParameters(t *testing.T) {
	m := manager(t)
	salt := []byte("0123456789abcdef")
	password := "correct horse battery"
	hash := argon2.IDKey([]byte(password), salt, 1, 1024, 1, 32)
	legacy := "$argon2id$v=19$m=1024,t=1,p=1$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(hash)
	if _, err := m.db.Exec("INSERT INTO settings(key,value) VALUES('password_hash',?)", legacy); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.1"), password); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := m.db.QueryRow("SELECT value FROM settings WHERE key='password_hash'").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got == legacy {
		t.Fatal("legacy PHC was not rehashed")
	}
}

func TestMalformedPHCIsRejected(t *testing.T) {
	m := manager(t)
	if _, err := m.db.Exec("INSERT INTO settings(key,value) VALUES('password_hash','not-a-phc')"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.1"), "correct horse battery"); err == nil {
		t.Fatal("malformed PHC accepted")
	}
}

func TestSessionIdleAndAbsoluteExpiry(t *testing.T) {
	m := manager(t)
	now := time.Now()
	m.now = func() time.Time { return now }
	if err := m.SetPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	id, csrf, _, err := m.Login(context.Background(), netip.MustParseAddr("10.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour + time.Second)
	if ok, _ := m.Check(context.Background(), id, csrf, false); ok {
		t.Fatal("idle session accepted")
	}
	id, csrf, _, err = m.Login(context.Background(), netip.MustParseAddr("10.0.0.1"), "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(absoluteLifetime + time.Second)
	if ok, _ := m.Check(context.Background(), id, csrf, false); ok {
		t.Fatal("absolute expiry accepted")
	}
}

func TestRevokeOtherSessions(t *testing.T) {
	m := manager(t)
	_ = m.SetPassword(context.Background(), "correct horse battery")
	a, csrfA, _, _ := m.Login(context.Background(), netip.MustParseAddr("10.0.0.1"), "correct horse battery")
	b, csrfB, _, _ := m.Login(context.Background(), netip.MustParseAddr("10.0.0.2"), "correct horse battery")
	m.RevokeOtherSessions(context.Background(), a)
	if ok, _ := m.Check(context.Background(), a, csrfA, false); !ok {
		t.Fatal("kept session revoked")
	}
	if ok, _ := m.Check(context.Background(), b, csrfB, false); ok {
		t.Fatal("other session remains")
	}
}
