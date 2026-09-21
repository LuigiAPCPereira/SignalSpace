package admin

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"
)

func TestPasswordKeyUsesArgon2idWithFixedCost(t *testing.T) {
	var salt [16]byte
	copy(salt[:], []byte("fixed-test-salt!"))
	got := passwordKey(testPassphrase, salt)
	want := argon2.IDKey([]byte(testPassphrase), salt[:], 3, 32*1024, 1, 32)
	if string(got[:]) != string(want) {
		t.Fatal("password KDF differs from the pinned Argon2id implementation")
	}
	if argonMemoryKiB < 19*1024 || argonPasses < 2 || argonThreads < 1 {
		t.Fatal("password KDF cost fell below documented minimum")
	}
}

func TestGateLimitsSessionCountWithoutConsumingBootstrap(t *testing.T) {
	g, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	bootstrap, _, err := g.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	first, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	g.mu.Lock()
	for i := 0; len(g.sessions) < maxSessionCount; i++ {
		key := sha256.Sum256([]byte(fmt.Sprintf("synthetic-session-%d", i)))
		g.sessions[key] = sessionRecord{csrf: "synthetic", idleUntil: now.Add(idleTTL), absoluteAt: now.Add(absoluteTTL)}
	}
	g.mu.Unlock()
	bootstrap, _, err = g.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Unlock(bootstrap.Cookie, bootstrap.CSRF, testPassphrase); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("unbounded session issuance: %v", err)
	}
	if _, err := g.Verify(first.Cookie, first.CSRF, true); err != nil {
		t.Fatalf("capacity failure revoked unrelated session: %v", err)
	}
	g.mu.Lock()
	for key, item := range g.sessions {
		if item.csrf == "synthetic" {
			delete(g.sessions, key)
			break
		}
	}
	g.mu.Unlock()
	if _, err := g.Unlock(bootstrap.Cookie, bootstrap.CSRF, testPassphrase); err != nil {
		t.Fatalf("capacity failure consumed bootstrap: %v", err)
	}
}

func TestInvalidPassphraseNeverPairsAndCountsUnlockFailures(t *testing.T) {
	g, code, err := NewGate()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	bootstrap, _, err := g.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"short", strings.Repeat("x", 129), string([]byte{0xff})} {
		if _, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, invalid); !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("invalid passphrase paired: %v", err)
		}
	}
	session, err := g.Pair(bootstrap.Cookie, bootstrap.CSRF, code, testPassphrase)
	if err != nil {
		t.Fatalf("invalid passphrase consumed pairing code: %v", err)
	}
	if err := g.Lock(session.Cookie, session.CSRF); err != nil {
		t.Fatal(err)
	}
	bootstrap, _, err = g.Bootstrap("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		_, err := g.Unlock(bootstrap.Cookie, bootstrap.CSRF, "short")
		if i < 4 && !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("failure %d not counted: %v", i+1, err)
		}
		if i == 4 && !errors.Is(err, ErrRateLimited) {
			t.Fatalf("fifth invalid passphrase did not lock out: %v", err)
		}
	}
	if _, err := g.Unlock(bootstrap.Cookie, bootstrap.CSRF, testPassphrase); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("valid password bypassed lockout: %v", err)
	}
}
