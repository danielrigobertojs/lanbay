// SPDX-License-Identifier: Apache-2.0
package auth

import (
	"context"
	"database/sql"
	_ "modernc.org/sqlite"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func manager(t *testing.T) *Manager {
	t.Helper()
	db, e := sql.Open("sqlite", ":memory:")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY,value TEXT NOT NULL)"); e != nil {
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
	if ok, _ := m.Check(id, "", true); ok {
		t.Fatal("missing CSRF accepted")
	}
	if ok, _ := m.Check(id, csrf, true); !ok {
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
	m.CompletePasswordChange(id)
	if ok, stillRequired := m.Check(id, csrf, false); !ok || stillRequired {
		t.Fatalf("session after password change = %t, %t", ok, stillRequired)
	}
}
