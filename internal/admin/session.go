package admin

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	pairingTTL        = 5 * time.Minute
	bootstrapTTL      = 5 * time.Minute
	idleTTL           = 15 * time.Minute
	absoluteTTL       = time.Hour
	unlockWindow      = 15 * time.Minute
	argonPasses       = 3
	argonMemoryKiB    = 32 * 1024
	argonThreads      = 1
	maxBootstrapCount = 64
	maxSessionCount   = 64
)

var (
	ErrAccessDenied  = errors.New("access denied")
	ErrRateLimited   = errors.New("rate limited")
	ErrAlreadyPaired = errors.New("already paired")
	ErrNotPaired     = errors.New("not paired")
	ErrUnavailable   = errors.New("temporarily unavailable")
)

type Bootstrap struct {
	Cookie    string
	CSRF      string
	ExpiresAt time.Time
}

type Session struct {
	Cookie     string
	CSRF       string
	IdleUntil  time.Time
	AbsoluteAt time.Time
}

type bootstrapRecord struct {
	csrf    string
	expires time.Time
}

type sessionRecord struct {
	csrf       string
	idleUntil  time.Time
	absoluteAt time.Time
}

// Gate mantém a identidade do proprietário apenas na memória do processo Quick.
// Nenhuma operação usa IP local, JWT OAuth ou cookie público como autenticação.
type Gate struct {
	mu           sync.Mutex
	now          func() time.Time
	pairHash     [32]byte
	pairUntil    time.Time
	pairFailures int
	paired       bool
	passwordSalt [16]byte
	passwordHash [32]byte
	unlockStart  time.Time
	unlockFails  int
	unlockUntil  time.Time
	bootstraps   map[[32]byte]bootstrapRecord
	sessions     map[[32]byte]sessionRecord
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func NewGate() (*Gate, string, error) {
	code, err := randomToken(24)
	if err != nil {
		return nil, "", err
	}
	g := &Gate{
		now:        time.Now,
		pairHash:   sha256.Sum256([]byte(code)),
		pairUntil:  time.Now().Add(pairingTTL),
		bootstraps: make(map[[32]byte]bootstrapRecord),
		sessions:   make(map[[32]byte]sessionRecord),
	}
	return g, code, nil
}

// passwordKey utiliza a implementação Argon2id do x/crypto, com parâmetros
// fixos e custo de memória limitado por tentativa. A instância serializa KDFs.
func passwordKey(passphrase string, salt [16]byte) [32]byte {
	password := []byte(passphrase)
	derived := argon2.IDKey(password, salt[:], argonPasses, argonMemoryKiB, argonThreads, 32)
	var result [32]byte
	copy(result[:], derived)
	clear(password)
	clear(derived)
	return result
}

func validPassphrase(passphrase string) bool {
	return utf8.ValidString(passphrase) && len(passphrase) >= 12 && len(passphrase) <= 128
}

func sameToken(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (g *Gate) cleanupLocked(now time.Time) {
	for key, item := range g.bootstraps {
		if !now.Before(item.expires) {
			delete(g.bootstraps, key)
		}
	}
	for key, item := range g.sessions {
		if !now.Before(item.idleUntil) || !now.Before(item.absoluteAt) {
			delete(g.sessions, key)
		}
	}
}

// Bootstrap nunca concede acesso aos pedidos OAuth e não prolonga sessão autenticada.
func (g *Gate) Bootstrap(cookie string) (Bootstrap, bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	if cookie != "" {
		if item, ok := g.bootstraps[sha256.Sum256([]byte(cookie))]; ok {
			return Bootstrap{Cookie: cookie, CSRF: item.csrf, ExpiresAt: item.expires}, g.paired, nil
		}
	}
	if len(g.bootstraps) >= maxBootstrapCount {
		return Bootstrap{}, g.paired, ErrRateLimited
	}
	id, err := randomToken(32)
	if err != nil {
		return Bootstrap{}, g.paired, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return Bootstrap{}, g.paired, err
	}
	item := bootstrapRecord{csrf: csrf, expires: now.Add(bootstrapTTL)}
	g.bootstraps[sha256.Sum256([]byte(id))] = item
	return Bootstrap{Cookie: id, CSRF: csrf, ExpiresAt: item.expires}, g.paired, nil
}

func (g *Gate) checkBootstrapLocked(cookie, csrf string, now time.Time) bool {
	item, ok := g.bootstraps[sha256.Sum256([]byte(cookie))]
	return cookie != "" && ok && now.Before(item.expires) && sameToken(csrf, item.csrf)
}

func (g *Gate) issueLocked(now time.Time) (Session, error) {
	if len(g.sessions) >= maxSessionCount {
		return Session{}, ErrRateLimited
	}
	id, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	item := sessionRecord{csrf: csrf, idleUntil: now.Add(idleTTL), absoluteAt: now.Add(absoluteTTL)}
	g.sessions[sha256.Sum256([]byte(id))] = item
	return Session{Cookie: id, CSRF: csrf, IdleUntil: item.idleUntil, AbsoluteAt: item.absoluteAt}, nil
}

// Pair é atômico: consome segredo e bootstrap somente depois de criar a sessão.
func (g *Gate) Pair(bootstrapCookie, csrf, code, passphrase string) (Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	if !g.checkBootstrapLocked(bootstrapCookie, csrf, now) {
		return Session{}, ErrAccessDenied
	}
	if g.paired {
		return Session{}, ErrAlreadyPaired
	}
	if !now.Before(g.pairUntil) {
		return Session{}, ErrAccessDenied
	}
	digest := sha256.Sum256([]byte(code))
	if subtle.ConstantTimeCompare(digest[:], g.pairHash[:]) != 1 {
		g.pairFailures++
		if g.pairFailures >= 3 {
			g.pairUntil = time.Time{}
		}
		return Session{}, ErrAccessDenied
	}
	if !validPassphrase(passphrase) {
		return Session{}, ErrAccessDenied
	}
	var salt [16]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return Session{}, err
	}
	key := passwordKey(passphrase, salt)
	session, err := g.issueLocked(now)
	if err != nil {
		return Session{}, err
	}
	g.passwordSalt = salt
	g.passwordHash = key
	g.paired = true
	g.pairHash = [32]byte{}
	g.pairUntil = time.Time{}
	delete(g.bootstraps, sha256.Sum256([]byte(bootstrapCookie)))
	return session, nil
}

// Unlock exige frase-senha e bootstrap; tentativas são limitadas por instância.
func (g *Gate) Unlock(bootstrapCookie, csrf, passphrase string) (Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	if !g.checkBootstrapLocked(bootstrapCookie, csrf, now) {
		return Session{}, ErrAccessDenied
	}
	if !g.paired {
		return Session{}, ErrNotPaired
	}
	if now.Before(g.unlockUntil) {
		return Session{}, ErrRateLimited
	}
	if g.unlockStart.IsZero() || !now.Before(g.unlockStart.Add(unlockWindow)) {
		g.unlockStart = now
		g.unlockFails = 0
	}
	key := passwordKey(passphrase, g.passwordSalt)
	valid := validPassphrase(passphrase) && subtle.ConstantTimeCompare(key[:], g.passwordHash[:]) == 1
	if !valid {
		g.unlockFails++
		if g.unlockFails >= 5 {
			g.unlockUntil = now.Add(unlockWindow)
			g.unlockFails = 0
			g.unlockStart = time.Time{}
			return Session{}, ErrRateLimited
		}
		return Session{}, ErrAccessDenied
	}
	session, err := g.issueLocked(now)
	if err != nil {
		return Session{}, err
	}
	g.unlockFails = 0
	g.unlockStart = time.Time{}
	delete(g.bootstraps, sha256.Sum256([]byte(bootstrapCookie)))
	return session, nil
}

// Verify confere estado no servidor; consultas nunca renovam prazo ocioso.
func (g *Gate) Verify(cookie, csrf string, requireCSRF bool) (Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	item, ok := g.sessions[sha256.Sum256([]byte(cookie))]
	if cookie == "" || !ok || (requireCSRF && !sameToken(csrf, item.csrf)) {
		return Session{}, ErrAccessDenied
	}
	return Session{Cookie: cookie, CSRF: item.csrf, IdleUntil: item.idleUntil, AbsoluteAt: item.absoluteAt}, nil
}

func (g *Gate) Refresh(cookie, csrf string) (Session, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	key := sha256.Sum256([]byte(cookie))
	item, ok := g.sessions[key]
	if cookie == "" || !ok || !sameToken(csrf, item.csrf) {
		return Session{}, ErrAccessDenied
	}
	item.idleUntil = now.Add(idleTTL)
	if item.idleUntil.After(item.absoluteAt) {
		item.idleUntil = item.absoluteAt
	}
	g.sessions[key] = item
	return Session{Cookie: cookie, CSRF: item.csrf, IdleUntil: item.idleUntil, AbsoluteAt: item.absoluteAt}, nil
}

func (g *Gate) Lock(cookie, csrf string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	g.cleanupLocked(now)
	key := sha256.Sum256([]byte(cookie))
	item, ok := g.sessions[key]
	if cookie == "" || !ok || !sameToken(csrf, item.csrf) {
		return ErrAccessDenied
	}
	delete(g.sessions, key)
	return nil
}

// ResetPairing é reservado a uma ação explícita do terminal, nunca HTTP.
func (g *Gate) ResetPairing() (string, error) {
	code, err := randomToken(24)
	if err != nil {
		return "", err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paired = false
	g.passwordHash = [32]byte{}
	g.passwordSalt = [16]byte{}
	g.pairHash = sha256.Sum256([]byte(code))
	g.pairUntil = g.now().Add(pairingTTL)
	g.pairFailures = 0
	g.unlockFails = 0
	g.unlockStart = time.Time{}
	g.unlockUntil = time.Time{}
	g.bootstraps = make(map[[32]byte]bootstrapRecord)
	g.sessions = make(map[[32]byte]sessionRecord)
	return code, nil
}

func (g *Gate) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.paired = false
	g.pairHash = [32]byte{}
	g.passwordHash = [32]byte{}
	g.passwordSalt = [16]byte{}
	g.bootstraps = make(map[[32]byte]bootstrapRecord)
	g.sessions = make(map[[32]byte]sessionRecord)
}
