package admin

import (
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"
)

const testPassphrase = "a-long-local-passphrase"

func TestGatePairCreatesAuthenticatedSessionAndRevokesOnLock(t *testing.T) {
	g, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	bootstrap, paired, err := g.Bootstrap("")
	if err != nil || paired {
		t.Fatalf("unexpected bootstrap: %v %v", paired, err)
	}
	if _, err := g.Verify(bootstrap.Cookie, bootstrap.CSRF, true); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("bootstrap authenticated as owner")
	}
	if _, err := g.Pair(bootstrap.Cookie, "invalid", code, testPassphrase); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("pairing accepted wrong CSRF")
	}
	session, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if session.Cookie == bootstrap.Cookie || session.CSRF == bootstrap.CSRF {
		t.Fatal("bootstrap credentials reused")
	}
	if _, err := g.Verify(session.Cookie, "", true); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("decision accepted without CSRF")
	}
	if _, err := g.Verify(session.Cookie, session.CSRF, true); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("replayed pairing")
	}
	if err := g.Lock(session.Cookie, session.CSRF); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Verify(session.Cookie, session.CSRF, true); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("locked session still authorized")
	}
	secondBootstrap, paired, err := g.Bootstrap("")
	if err != nil || !paired {
		t.Fatal("lost pairing after lock")
	}
	second, err := g.Unlock(secondBootstrap.Cookie, secondBootstrap.CSRF, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	if second.Cookie == session.Cookie || second.CSRF == session.CSRF {
		t.Fatal("reused old session")
	}
	g.Close()
	if _, err := g.Verify(second.Cookie, second.CSRF, true); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("shutdown did not revoke session")
	}
}

func TestGateExpiryAndExplicitRefresh(t *testing.T) {
	g, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	now := time.Now()
	g.now = func() time.Time { return now }
	bootstrap, _, _ := g.Bootstrap("")
	session, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(14 * time.Minute)
	observed, err := g.Verify(session.Cookie, session.CSRF, true)
	if err != nil || !observed.IdleUntil.Equal(session.IdleUntil) {
		t.Fatal("GET renewed idle session")
	}
	refreshed, err := g.Refresh(session.Cookie, session.CSRF)
	if err != nil || !refreshed.IdleUntil.After(observed.IdleUntil) {
		t.Fatal("explicit refresh did not renew idle")
	}
	for _, minute := range []int{28, 42, 56} {
		now = session.AbsoluteAt.Add(time.Duration(minute-60) * time.Minute)
		refreshed, err = g.Refresh(session.Cookie, session.CSRF)
		if err != nil {
			t.Fatalf("refresh at minute %d: %v", minute, err)
		}
	}
	if !refreshed.IdleUntil.Equal(session.AbsoluteAt) {
		t.Fatal("refresh extended beyond absolute expiration")
	}
	now = session.AbsoluteAt
	if _, err := g.Verify(session.Cookie, "", false); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("absolute expiration accepted")
	}
}

func TestGateFailedPairingAndRecovery(t *testing.T) {
	g, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	bootstrap, _, _ := g.Bootstrap("")
	for i := 0; i < 3; i++ {
		if _, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, "wrong", testPassphrase); !errors.Is(err, ErrAccessDenied) {
			t.Fatal("wrong pairing accepted")
		}
	}
	if _, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("pairing code survived third failure")
	}
	newCode, err := g.ResetPairing()
	if err != nil || newCode == code {
		t.Fatal("reset did not rotate pairing code")
	}
	bootstrap, _, _ = g.Bootstrap("")
	if _, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, newCode, testPassphrase); err != nil {
		t.Fatal(err)
	}
}

func TestGateSingleWinnerInConcurrentPairing(t *testing.T) {
	g, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	bootstrap, _, _ := g.Bootstrap("")
	var group sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, ErrAccessDenied) && !errors.Is(err, ErrAlreadyPaired) {
			t.Fatalf("unexpected pairing result: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("pairing applied %d times", success)
	}
}

func TestPasswordKDFDeterministicSalted(t *testing.T) {
	var salt [16]byte
	first := passwordKey(testPassphrase, salt)
	second := passwordKey(testPassphrase, salt)
	if first != second || first == sha256.Sum256([]byte(testPassphrase)) {
		t.Fatal("unexpected KDF output")
	}
	salt[0] = 1
	if passwordKey(testPassphrase, salt) == first {
		t.Fatal("salt did not change key")
	}
}
