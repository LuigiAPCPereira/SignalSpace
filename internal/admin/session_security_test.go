package admin

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestGateUnlockRateLimitAndRecovery(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	now := time.Now()
	gate.now = func() time.Time { return now }
	bootstrap, _, err := gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	session, err := gate.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if err := gate.Lock(session.Cookie, session.CSRF); err != nil {
		t.Fatal(err)
	}
	bootstrap, _, err = gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		if _, err := gate.Unlock(bootstrap.Cookie, bootstrap.CSRF, "incorrect-passphrase"); !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if _, err := gate.Unlock(bootstrap.Cookie, bootstrap.CSRF, testPassphrase); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("valid password bypassed lockout: %v", err)
	}
	now = now.Add(16 * time.Minute)
	bootstrap, _, err = gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Unlock(bootstrap.Cookie, bootstrap.CSRF, testPassphrase); err != nil {
		t.Fatalf("unlock after cooldown: %v", err)
	}
}

func TestGateExpiredBootstrapAndCrossSessionCSRF(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	now := time.Now()
	gate.now = func() time.Time { return now }
	first, _, err := gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Pair(first.Cookie, other.CSRF, code, testPassphrase); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("cross-session CSRF accepted")
	}
	now = now.Add(6 * time.Minute)
	if _, err := gate.Pair(first.Cookie, first.CSRF, code, testPassphrase); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("expired bootstrap accepted")
	}
	newCode, err := gate.ResetPairing()
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, err := gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gate.Pair(fresh.Cookie, fresh.CSRF, newCode, testPassphrase); err != nil {
		t.Fatal(err)
	}
}

func TestGateConcurrentVerificationAfterLockReturns(t *testing.T) {
	gate, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	bootstrap, _, err := gate.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	session, err := gate.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 24
	var group sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := gate.Verify(session.Cookie, session.CSRF, true)
			results <- err
		}()
	}
	if err := gate.Lock(session.Cookie, session.CSRF); err != nil {
		t.Fatal(err)
	}
	group.Wait()
	for i := 0; i < workers; i++ {
		if err := <-results; err != nil && !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := gate.Verify(session.Cookie, session.CSRF, true)
			results <- err
		}()
	}
	group.Wait()
	for i := 0; i < workers; i++ {
		if err := <-results; !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("old cookie accepted after lock returned: %v", err)
		}
	}
}
