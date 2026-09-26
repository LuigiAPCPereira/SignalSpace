// Package policy implementa o núcleo determinístico de decisão local.
//
// O engine é deliberadamente independente do transporte MCP e de grants de
// filesystem. Nesta fatia ele fornece a decisão e, somente quando o resultado
// é REQUIRE_APPROVAL, pode delegar a criação de um pedido local explícito.
// Permits, persistência e tradução para uma resposta pública continuam fora
// deste domínio.
package policy

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
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
	ErrInvalidPolicyContext        = errors.New("invalid policy context")
	ErrInvalidPolicyRule           = errors.New("invalid policy rule")
	ErrUnsupportedEffect           = errors.New("unsupported policy effect")
	ErrManagedWorkspaceUnavailable = errors.New("managed workspace is unavailable")
)

// Context contém os vínculos que uma decisão precisa revalidar. Campos
// ausentes não são tratados como curingas: Evaluate falha fechado.
type Context struct {
	OwnerID             string
	ClientID            string
	TokenFamilyID       string
	WorkspaceID         string
	ManagedWorkspaceID  string
	SessionID           string
	Capability          capability.Capability
	Tool                string
	Fingerprint         string
	GrantActive         bool
	GrantedCapabilities []capability.Capability
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
	mu              sync.RWMutex
	now             func() time.Time
	rules           []Rule
	store           *Store
	workspaceStable func(string) bool
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

// NewEngineWithStore carrega somente políticas ALLOW_WORKSPACE válidas. Um
// store inválido impede a composição: continuar com regras parcialmente
// carregadas transformaria corrupção em autorização implícita.
func NewEngineWithStore(now func() time.Time, store *Store, workspaceStable func(string) bool) (*Engine, error) {
	if store == nil || workspaceStable == nil {
		return nil, ErrInvalidPolicyRule
	}
	if now == nil {
		now = time.Now
	}
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	engine := &Engine{now: now, store: store, workspaceStable: workspaceStable}
	for _, item := range items {
		if !workspaceStable(item.WorkspaceID) {
			continue
		}
		engine.rules = append(engine.rules, Rule{
			OwnerID: item.OwnerID, ClientID: item.ClientID, WorkspaceID: item.WorkspaceID,
			Capability: item.Capability, Effect: EffectAllowWorkspace,
		})
	}
	return engine, nil
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

// Evaluate retorna sempre uma decisão tipada. Contexto inválido, grant
// inativo, capability desconhecida, ausência de regra e regra expirada
// resultam em DENY. Um contexto válido com capability ainda não concedida
// retorna REQUIRE_APPROVAL, sem criar fila ou efeito externo.
func (e *Engine) Evaluate(ctx Context) Decision {
	if e == nil || !validContext(ctx) {
		return Deny
	}
	if !ctx.GrantActive {
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
		if rule.Effect == EffectAllowWorkspace && e.workspaceStable != nil && !e.workspaceStable(rule.WorkspaceID) {
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
	if bestIndex >= 0 {
		switch e.rules[bestIndex].Effect {
		case EffectDeny:
			return Deny
		case EffectAsk:
			return RequireApproval
		case EffectAllowSession, EffectAllowWorkspace:
			if !hasGrantedCapability(ctx) {
				return RequireApproval
			}
			return Allow
		default:
			return Deny
		}
	}
	if !hasGrantedCapability(ctx) {
		return RequireApproval
	}
	return Deny
}

// ApplyApproval traduz uma decisão administrativa em uma política local. O
// chamador deve manter o lifecycle da fila; este método não executa operação.
func (e *Engine) ApplyApproval(snapshot approval.Snapshot, decision approval.Decision) error {
	if e == nil {
		return ErrInvalidPolicyRule
	}
	switch decision {
	case approval.DecisionDeny, approval.DecisionAllowOnce:
		return nil
	case approval.DecisionAllowSession:
		return e.SetRule(Rule{OwnerID: snapshot.OwnerID, ClientID: snapshot.ClientID, WorkspaceID: snapshot.WorkspaceID, SessionID: snapshot.SessionID, Capability: snapshot.Capability, Effect: EffectAllowSession})
	case approval.DecisionAllowWorkspace:
		if e.store == nil || snapshot.ManagedWorkspaceID == "" || e.workspaceStable == nil || !e.workspaceStable(snapshot.ManagedWorkspaceID) {
			return ErrManagedWorkspaceUnavailable
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		item := StoredPolicy{OwnerID: snapshot.OwnerID, ClientID: snapshot.ClientID, WorkspaceID: snapshot.ManagedWorkspaceID, Capability: snapshot.Capability, Effect: EffectAllowWorkspace}
		stored, err := e.store.Upsert(item)
		if err != nil {
			return err
		}
		rule := Rule{OwnerID: stored.OwnerID, ClientID: stored.ClientID, WorkspaceID: stored.WorkspaceID, Capability: stored.Capability, Effect: EffectAllowWorkspace}
		for index, existing := range e.rules {
			if sameSelectors(existing, rule) {
				e.rules[index] = rule
				return nil
			}
		}
		e.rules = append(e.rules, rule)
		return nil
	default:
		return ErrUnsupportedEffect
	}
}

func (e *Engine) ListPolicies() ([]StoredPolicy, error) {
	if e == nil || e.store == nil {
		return nil, ErrStoreUnavailable
	}
	return e.store.List()
}

func (e *Engine) RevokePolicy(id string) error {
	if e == nil || e.store == nil {
		return ErrStoreUnavailable
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	items, err := e.store.List()
	if err != nil {
		return err
	}
	var target StoredPolicy
	for _, item := range items {
		if item.ID == id {
			target = item
			break
		}
	}
	if target.ID == "" {
		return ErrPolicyNotFound
	}
	if err := e.store.Delete(id); err != nil {
		return err
	}
	for index := range e.rules {
		if e.rules[index].OwnerID == target.OwnerID && e.rules[index].ClientID == target.ClientID && e.rules[index].WorkspaceID == target.WorkspaceID && e.rules[index].Capability == target.Capability && e.rules[index].Effect == EffectAllowWorkspace {
			e.rules = append(e.rules[:index], e.rules[index+1:]...)
			break
		}
	}
	return nil
}

// ApprovalRequester é a única ponte interna deste gate. Ela não é utilizada
// pelo handler MCP público e não executa a operação original.
type ApprovalRequester interface {
	Create(approval.RequestInput) (approval.Snapshot, bool, error)
}

// EvaluateAndRequest preserva a semântica de DENY e ALLOW. Apenas
// REQUIRE_APPROVAL cria/reusa um pedido, permitindo que um harness local
// exercite a fila sem expor approvals pelo transporte público.
func (e *Engine) EvaluateAndRequest(ctx Context, requester ApprovalRequester, safeSummary string) (Decision, approval.Snapshot, bool, error) {
	decision := e.Evaluate(ctx)
	if decision != RequireApproval {
		return decision, approval.Snapshot{}, false, nil
	}
	if requester == nil {
		return decision, approval.Snapshot{}, false, errors.New("approval requester unavailable")
	}
	allowed := []approval.Decision{approval.DecisionAllowOnce, approval.DecisionAllowSession, approval.DecisionDeny}
	if ctx.ManagedWorkspaceID != "" && e.workspaceStable != nil && e.workspaceStable(ctx.ManagedWorkspaceID) {
		allowed = append(allowed, approval.DecisionAllowWorkspace)
	}
	snapshot, reused, err := requester.Create(approval.RequestInput{
		OwnerID: ctx.OwnerID, ClientID: ctx.ClientID, TokenFamilyID: ctx.TokenFamilyID,
		WorkspaceID: ctx.WorkspaceID, ManagedWorkspaceID: ctx.ManagedWorkspaceID, SessionID: ctx.SessionID, Capability: ctx.Capability,
		Tool: ctx.Tool, OperationFingerprint: ctx.Fingerprint, SafeSummary: safeSummary, AllowedDecisions: allowed,
	})
	if err != nil {
		return decision, approval.Snapshot{}, false, err
	}
	return decision, snapshot, reused, nil
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
	if ctx.OwnerID == "" || ctx.ClientID == "" || ctx.TokenFamilyID == "" ||
		ctx.WorkspaceID == "" || ctx.SessionID == "" || !capability.IsKnown(ctx.Capability) ||
		ctx.Tool == "" || ctx.Fingerprint == "" {
		return false
	}
	for _, granted := range ctx.GrantedCapabilities {
		if !capability.IsKnown(granted) {
			return false
		}
	}
	return true
}

func hasGrantedCapability(ctx Context) bool {
	for _, granted := range ctx.GrantedCapabilities {
		if granted == ctx.Capability {
			return true
		}
	}
	return false
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
