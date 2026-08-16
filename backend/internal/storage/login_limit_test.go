package storage

import (
	"testing"
	"time"

	"fuze-ai-paas/backend/internal/models"
)

func seedLocalUser(t *testing.T, s *Storage, username string) {
	t.Helper()
	u, err := s.UpsertSSOUser(&models.User{
		ID:       "u-" + username,
		Username: username,
		Email:    username + "@example.com",
		Role:     models.RoleDeveloper,
		TenantID: "default",
		Password: "$2a$10$test", 
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_ = u
}

func TestRecordLoginFailureLocksAfterThreshold(t *testing.T) {
	s := newEntStorage(t)
	seedLocalUser(t, s, "locky")

	const maxFails = 3
	const lockSec = 900
	
	for i := 1; i <= maxFails-1; i++ {
		u, locked, err := s.RecordLoginFailure("locky", maxFails, lockSec)
		if err != nil {
			t.Fatal(err)
		}
		if locked {
			t.Fatalf("attempt %d: should not be locked yet", i)
		}
		if u.LoginFails != i {
			t.Fatalf("attempt %d: expected LoginFails=%d, got %d", i, i, u.LoginFails)
		}
		if u.LockedUntil != nil {
			t.Fatalf("attempt %d: LockedUntil should be nil", i)
		}
	}

	u, locked, err := s.RecordLoginFailure("locky", maxFails, lockSec)
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("expected account to be locked at threshold")
	}
	if u.LockedUntil == nil || !u.LockedUntil.After(time.Now()) {
		t.Fatalf("expected future LockedUntil, got %v", u.LockedUntil)
	}

	u2, _ := s.GetUserByUsername("locky")
	if u2.LoginFails != 0 {
		t.Fatalf("after lock, LoginFails should reset to 0, got %d", u2.LoginFails)
	}
}

func TestClearLoginFailures(t *testing.T) {
	s := newEntStorage(t)
	seedLocalUser(t, s, "cleary")

	if _, _, err := s.RecordLoginFailure("cleary", 5, 900); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearLoginFailures("cleary"); err != nil {
		t.Fatal(err)
	}
	u, err := s.GetUserByUsername("cleary")
	if err != nil {
		t.Fatal(err)
	}
	if u.LoginFails != 0 {
		t.Fatalf("expected LoginFails=0 after clear, got %d", u.LoginFails)
	}
	if u.LockedUntil != nil {
		t.Fatalf("expected nil LockedUntil after clear, got %v", u.LockedUntil)
	}
}

func TestRecordLoginFailureDisabledLimit(t *testing.T) {
	s := newEntStorage(t)
	seedLocalUser(t, s, "nolimit")
	
	for i := 0; i < 10; i++ {
		u, locked, err := s.RecordLoginFailure("nolimit", 0, 900)
		if err != nil {
			t.Fatal(err)
		}
		if locked {
			t.Fatal("limit disabled: should never lock")
		}
		if u.LoginFails != i+1 {
			t.Fatalf("expected LoginFails=%d, got %d", i+1, u.LoginFails)
		}
	}
}