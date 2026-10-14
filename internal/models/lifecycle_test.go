package models

import (
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	u := User{Status: UserStatusPending, ReferralSource: "test", IntendedUse: "test", AcceptedTerms: true}
	if _, e := u.Transition("approve", 30, now); e != nil {
		t.Fatal(e)
	}
	expiry := *u.ExpiresAt
	if changed, e := u.Transition("approve", 30, now); e != nil || changed || !expiry.Equal(*u.ExpiresAt) {
		t.Fatal("repeated approval changed subscription")
	}
	if _, e := u.Transition("suspend", 0, now); e != nil {
		t.Fatal(e)
	}
	if u.IsSubscriptionActive(now) {
		t.Fatal("suspended access accepted")
	}
	if _, e := u.Transition("activate", 0, expiry); e == nil {
		t.Fatal("expired suspension activated")
	}
	u.Status = UserStatusExpired
	if _, e := u.Transition("extend", 10, expiry); e != nil || !u.IsSubscriptionActive(expiry) {
		t.Fatal("explicit extension failed")
	}
	u.Transition("revoke", 0, now)
	if _, e := u.Transition("activate", 0, now); e == nil {
		t.Fatal("revoked credential activated")
	}
	u.Transition("rotate", 0, now)
	if _, e := u.Transition("activate", 0, now); e != nil {
		t.Fatal(e)
	}
}
func TestTransitionsReject(t *testing.T) {
	now := time.Now()
	for _, status := range []UserStatus{UserStatusPending, UserStatusRejected, UserStatusRevoked, UserStatusExpired} {
		u := User{Status: status}
		if _, e := u.Transition("activate", 0, now); e == nil {
			t.Fatalf("activated %s", status)
		}
	}
	u := User{Status: UserStatusPending}
	if _, e := u.Transition("approve", 30, now); e == nil {
		t.Fatal("unregistered approved")
	}
	for _, d := range []int{-1, 0, 3651} {
		if _, e := u.Transition("extend", d, now); e == nil {
			t.Fatal("bad duration accepted")
		}
	}
}

func TestExpiryDayBoundary(t *testing.T) {
	now := time.Now()
	for _, tc := range []struct {
		d    time.Duration
		want int
	}{{0, 0}, {24 * time.Hour, 1}, {24*time.Hour + time.Nanosecond, 2}, {time.Hour, 1}, {-time.Hour, 0}} {
		expiry := now.Add(tc.d)
		u := User{Status: UserStatusActive, ExpiresAt: &expiry}
		if got := u.DaysUntilExpiry(now); got != tc.want {
			t.Fatalf("expiry days: got %d want %d", got, tc.want)
		}
	}
}
