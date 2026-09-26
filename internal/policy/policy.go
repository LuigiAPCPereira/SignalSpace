// Package policy implementa o núcleo determinístico de decisão local.
//
// O engine é deliberadamente independente do transporte MCP e de grants de
// filesystem. Nesta fatia ele fornece apenas a decisão; approvals, permits,
// persistência e a tradução para uma resposta pública são gates posteriores.
package policy

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

type Decision string

const (
	Allow           Decision = "ALLOW"
	Deny            Decision = "DENY"
	RequireApproval Decision = "REQUIRE_APPROVAL"
)

type Effect string

const (
	EffectDeny           Effect = "DENY"
	EffectAsk            Effect = "ASK"
	EffectAllowSession   Effect = "ALLOW_SESSION"
	EffectAllowWorkspace Effect = "ALLOW_WORKSPACE"
)

var (
	ErrInvalidPolicyContext = errors.New("invalid policy context")
	ErrInvalidPolicyRule    = errors.New("invalid policy rule")
	ErrUnsupportedEffect    = errors.New("unsupported policy effect")
)

// Context contém os vínculos que uma decisão precisa revalidar. Campos
// ausentes não são tratados como curingas: Evaluate falha fechado.
type Context struct {
	OwnerID       string
	ClientID      string
	TokenFamilyID string
	WorkspaceID   string
	SessionID     string
	Capability    capability.Capability
	Tool          string
	Fingerprint   string
}

// Rule usa campos não vazios como seletores exatos. OwnerID e Capability são
// sempre obrigatórios; os demais permitem políticas progressivamente mais
// específicas sem introduzir wildcard implícito.
type Rule struct {
	OwnerID       string
	ClientID      string
	TokenFamilyID string
	WorkspaceID   string
	SessionID     string
	Capability    capability.Capability
	Tool          string
	Fingerprint   string
	Effect        Effect
	ExpiresAt     time.Time
}

type Engine struct {
	mu    sync.RWMutex
	now   func() time.Time
	rules []Rule
}

func NewEngine() *Engine {
	return NewEngineWithClock(time.Now)
}

func NewEngineWithClock(now func() time.Time) *Engine {
	if now == nil {
		now = time.Now
	}
	return &Engine{now: now}
}

// SetRule substitui uma regra com os mesmos seletores e capability. A regra
// não pode criar uma capability desconhecida nem o futuro ALLOW_ONCE.
func (e *Engine) SetRule(rule Rule) error {
	if e == nil {
		return ErrInvalidPolicyRule
	}
	if err := validateRule(rule); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for index, existing := range e.rules {
		if sameSelectors(existing, rule) {
			e.rules[index] = rule
			return nil
		}
	}
	e.rules = append(e.rules, rule)
	return nil
}

// RemoveRule remove somente uma regra com os mesmos seletores e capability.
// A ausência é segura e não altera a decisão de uma regra diferente.
func (e *Engine) RemoveRule(rule Rule) error {
	if e == nil {
		return ErrInvalidPolicyRule
	}
	if err := validateRule(rule); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for index, existing := range e.rules {
		if sameSelectors(existing, rule) {
			e.rules = append(e.rules[:index], e.rules[index+1:]...)
			return nil
		}
	}
	return nil
}

// Evaluate retorna sempre uma decisão tipada. Contexto inválido, capability
// desconhecida, ausência de regra e regra expirada resultam em DENY.
func (e *Engine) Evaluate(ctx Context) Decision {
	if e == nil || !validContext(ctx) {
		return Deny
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	bestIndex := -1
	bestSpecificity := -1
	for index, rule := range e.rules {
		if rule.ExpiresAt != (time.Time{}) && !now.Before(rule.ExpiresAt) {
			continue
		}
		if !matches(rule, ctx) {
			continue
		}
		specificity := specificity(rule)
		if specificity > bestSpecificity ||
			(specificity == bestSpecificity && bestIndex >= 0 && rule.Effect == EffectDeny && e.rules[bestIndex].Effect != EffectDeny) ||
			(specificity == bestSpecificity && bestIndex >= 0 && rule.Effect == e.rules[bestIndex].Effect && index > bestIndex) {
			bestIndex = index
			bestSpecificity = specificity
		}
	}
	if bestIndex < 0 {
		return Deny
	}
	switch e.rules[bestIndex].Effect {
	case EffectDeny:
		return Deny
	case EffectAsk:
		return RequireApproval
	case EffectAllowSession, EffectAllowWorkspace:
		return Allow
	default:
		return Deny
	}
}

func validateRule(rule Rule) error {
	if strings.TrimSpace(rule.OwnerID) != rule.OwnerID || rule.OwnerID == "" ||
		!capability.IsKnown(rule.Capability) {
		return ErrInvalidPolicyRule
	}
	switch rule.Effect {
	case EffectDeny, EffectAsk:
	case EffectAllowSession:
		if rule.WorkspaceID == "" || rule.SessionID == "" {
			return ErrInvalidPolicyRule
		}
	case EffectAllowWorkspace:
		if rule.WorkspaceID == "" || rule.SessionID != "" {
			return ErrInvalidPolicyRule
		}
	default:
		return ErrUnsupportedEffect
	}
	return nil
}

func validContext(ctx Context) bool {
	return ctx.OwnerID != "" && ctx.ClientID != "" && ctx.TokenFamilyID != "" &&
		ctx.WorkspaceID != "" && ctx.SessionID != "" && capability.IsKnown(ctx.Capability) &&
		ctx.Tool != "" && ctx.Fingerprint != ""
}

func sameSelectors(left, right Rule) bool {
	return left.OwnerID == right.OwnerID && left.ClientID == right.ClientID &&
		left.TokenFamilyID == right.TokenFamilyID && left.WorkspaceID == right.WorkspaceID &&
		left.SessionID == right.SessionID && left.Capability == right.Capability &&
		left.Tool == right.Tool && left.Fingerprint == right.Fingerprint
}

func matches(rule Rule, ctx Context) bool {
	return rule.OwnerID == ctx.OwnerID && (rule.ClientID == "" || rule.ClientID == ctx.ClientID) &&
		(rule.TokenFamilyID == "" || rule.TokenFamilyID == ctx.TokenFamilyID) &&
		(rule.WorkspaceID == "" || rule.WorkspaceID == ctx.WorkspaceID) &&
		(rule.SessionID == "" || rule.SessionID == ctx.SessionID) &&
		rule.Capability == ctx.Capability && (rule.Tool == "" || rule.Tool == ctx.Tool) &&
		(rule.Fingerprint == "" || rule.Fingerprint == ctx.Fingerprint)
}

func specificity(rule Rule) int {
	value := 1 // owner is required and is the broadest valid scope.
	if rule.ClientID != "" {
		value += 2
	}
	if rule.TokenFamilyID != "" {
		value += 4
	}
	if rule.WorkspaceID != "" {
		value += 8
	}
	if rule.SessionID != "" {
		value += 16
	}
	if rule.Tool != "" {
		value += 32
	}
	if rule.Fingerprint != "" {
		value += 64
	}
	return value
}
