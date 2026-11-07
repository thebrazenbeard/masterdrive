package grantstate

import (
	"errors"
	"testing"
	"time"
)

func TestLifecycleFailClosed(t *testing.T) {
	now := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	s := New(clock)
	exp := now.Add(time.Hour)
	if err := s.Propose("g1", "fingerprint1", "share1", exp); err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize("g1", "fingerprint1", "share1", 0); !errors.Is(err, ErrDenied) {
		t.Fatalf("pending: %v", err)
	}
	epoch, err := s.Approve("g1")
	if err != nil || epoch != 1 {
		t.Fatalf("approve: %d %v", epoch, err)
	}
	for _, tc := range []struct {
		id, key, share string
		epoch          uint64
	}{
		{"wrong", "fingerprint1", "share1", epoch},
		{"g1", "wrong", "share1", epoch},
		{"g1", "fingerprint1", "wrong", epoch},
		{"g1", "fingerprint1", "share1", 0},
	} {
		if err := s.Authorize(tc.id, tc.key, tc.share, tc.epoch); !errors.Is(err, ErrDenied) {
			t.Fatalf("unauthorized: %+v %v", tc, err)
		}
	}
	if err := s.Authorize("g1", "fingerprint1", "share1", epoch); err != nil {
		t.Fatal(err)
	}
	s.SetAvailable(false)
	if err := s.Authorize("g1", "fingerprint1", "share1", epoch); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err := s.Revoke("g1"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	s.SetAvailable(true)
	next, err := s.Revoke("g1")
	if err != nil || next != 2 {
		t.Fatalf("revoke: %d %v", next, err)
	}
	if err := s.Authorize("g1", "fingerprint1", "share1", next); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if _, err := s.Approve("g1"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Propose("g1", "fingerprint1", "share1", exp); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.Propose("g2", "fingerprint1", "share1", exp); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve("g2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize("g2", "fingerprint1", "share1", 3); err != nil {
		t.Fatal(err)
	}
	now = exp
	if err := s.Authorize("g2", "fingerprint1", "share1", 3); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
}
func TestExpiredAndInvalidProposal(t *testing.T) {
	now := time.Unix(100, 0)
	s := New(func() time.Time { return now })
	for _, g := range []struct {
		id, key, share string
		exp            time.Time
	}{
		{"", "k", "s", now.Add(time.Hour)}, {"id", "", "s", now.Add(time.Hour)},
		{"id", "k", "", now.Add(time.Hour)}, {"id", "k", "s", now},
	} {
		if err := s.Propose(g.id, g.key, g.share, g.exp); !errors.Is(err, ErrInvalid) {
			t.Fatal(g, err)
		}
	}
	if err := s.Propose("g", "k", "s", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	if _, err := s.Approve("g"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
func TestConcurrentRevokeDenies(t *testing.T) {
	now := time.Unix(100, 0)
	s := New(func() time.Time { return now })
	if err := s.Propose("g", "k", "s", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve("g"); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = s.Revoke("g") }()
	<-done
	for i := 0; i < 100; i++ {
		if err := s.Authorize("g", "k", "s", 2); !errors.Is(err, ErrDenied) {
			t.Fatal(err)
		}
	}
}

func TestClockRollbackPoisonsGrantAuthority(t *testing.T) {
	now := time.Unix(1000, 0)
	s := New(func() time.Time { return now })
	expiry := now.Add(time.Hour)
	if err := s.Propose("g", "k", "s", expiry); err != nil {
		t.Fatal(err)
	}
	epoch, err := s.Approve("g")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Authorize("g", "k", "s", epoch); err != nil {
		t.Fatal(err)
	}
	now = now.Add(-time.Second)
	if err := s.Authorize("g", "k", "s", epoch); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("rollback: %v", err)
	}
	now = now.Add(2 * time.Second)
	if err := s.Authorize("g", "k", "s", epoch); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("rollback latch: %v", err)
	}
	if _, err := s.Revoke("g"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revoke after rollback: %v", err)
	}
	if err := s.Propose("g2", "k", "s", expiry); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("propose after rollback: %v", err)
	}
}
