// Package approval implementa o lifecycle local de pedidos de capability.
//
// Este domínio é deliberadamente separado de OAuth, grants e transporte MCP.
// Um permit é uma autorização interna de uso único por referência; não é um
// bearer, scope ou credencial que possa ser exposta pela API administrativa.
package approval

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

const (
	DefaultRequestTTL        = 2 * time.Minute
	DefaultPermitTTL         = 30 * time.Second
	DefaultTerminalRetention = 10 * time.Minute
	DefaultMaxPending        = 64
	DefaultMaxTerminal       = 64
	DefaultMaxSafeSummary    = 256
	maximumIdentifierLength  = 256
	maximumToolLength        = 128
)

var (
	ErrNotFound           = errors.New("approval request not found")
	ErrExpired            = errors.New("approval request expired")
	ErrStaleVersion       = errors.New("approval request version changed")
	ErrAlreadyDecided     = errors.New("approval request already decided")
	ErrInvalidDecision    = errors.New("invalid approval decision")
	ErrInvalidCapability  = errors.New("invalid approval capability")
	ErrInvalidFingerprint = errors.New("invalid operation fingerprint")
	ErrInvalidSummary     = errors.New("invalid approval summary")
	ErrCapacityExceeded   = errors.New("approval capacity exceeded")
	ErrClosed             = errors.New("approval service closed")
	ErrPermitNotFound     = errors.New("operation permit not found")
	ErrPermitExpired      = errors.New("operation permit expired")
	ErrPermitConsumed     = errors.New("operation permit already consumed")
	ErrPermitContext      = errors.New("operation permit context mismatch")
	ErrPermitAmbiguous    = errors.New("multiple operation permits match context")
	ErrPermitRevoked      = errors.New("operation permit grant revoked")
	ErrInvalidInput       = errors.New("invalid approval request")
	ErrInvalidConfig      = errors.New("invalid approval configuration")
)

type Status string

const (
	StatusPending  Status = "PENDING"
	StatusApproved Status = "APPROVED"
	StatusDenied   Status = "DENIED"
	StatusExpired  Status = "EXPIRED"
)

type Decision string

const (
	DecisionAllowOnce      Decision = "ALLOW_ONCE"
	DecisionAllowSession   Decision = "ALLOW_SESSION"
	DecisionAllowWorkspace Decision = "ALLOW_WORKSPACE"
	DecisionDeny           Decision = "DENY"
)

// RequestInput contém a autoridade da operação. SafeSummary é somente
// apresentação e nunca participa da deduplicação ou do consumo.
type RequestInput struct {
	OwnerID              string
	ClientID             string
	TokenFamilyID        string
	WorkspaceID          string
	ManagedWorkspaceID   string
	SessionID            string
	AllowedDecisions     []Decision
	Capability           capability.Capability
	Tool                 string
	OperationFingerprint string
	SafeSummary          string
}

// Snapshot é a representação administrativa segura de um pedido.
type Snapshot struct {
	RequestID            string                `json:"request_id"`
	OwnerID              string                `json:"owner_id"`
	ClientID             string                `json:"client_id"`
	TokenFamilyID        string                `json:"token_family_id,omitempty"`
	WorkspaceID          string                `json:"workspace_id"`
	ManagedWorkspaceID   string                `json:"managed_workspace_id,omitempty"`
	SessionID            string                `json:"session_id"`
	Capability           capability.Capability `json:"capability"`
	Tool                 string                `json:"tool"`
	OperationFingerprint string                `json:"operation_fingerprint"`
	SafeSummary          string                `json:"safe_summary"`
	Status               Status                `json:"status"`
	Version              int                   `json:"version"`
	CreatedAt            time.Time             `json:"created_at"`
	ExpiresAt            time.Time             `json:"expires_at"`
	DecidedAt            *time.Time            `json:"decided_at,omitempty"`
	AllowedDecisions     []Decision            `json:"allowed_decisions"`
}

// Permit é uma referência interna de uso único. Ele não contém segredo e não
// é retornado pela rota administrativa; o consumidor futuro receberá apenas
// este valor por uma porta interna autenticada.
type Permit struct {
	PermitID             string
	RequestID            string
	OwnerID              string
	ClientID             string
	TokenFamilyID        string
	WorkspaceID          string
	SessionID            string
	Capability           capability.Capability
	Tool                 string
	OperationFingerprint string
	CreatedAt            time.Time
	ExpiresAt            time.Time
	ConsumedAt           *time.Time
}

type DecisionResult struct {
	Request Snapshot
	Permit  *Permit
}

// ConsumeContext é revalidado no instante do consumo. GrantActive representa
// a checagem feita pelo serviço de autorização proprietário do grant/session.
type ConsumeContext struct {
	OwnerID              string
	ClientID             string
	TokenFamilyID        string
	WorkspaceID          string
	SessionID            string
	Capability           capability.Capability
	Tool                 string
	OperationFingerprint string
	GrantActive          bool
}

type Config struct {
	RequestTTL        time.Duration
	PermitTTL         time.Duration
	TerminalRetention time.Duration
	MaxPending        int
	MaxTerminal       int
	MaxSafeSummary    int
	BeforeDecision    func(Snapshot, Decision) error
}

func DefaultConfig() Config {
	return Config{
		RequestTTL:        DefaultRequestTTL,
		PermitTTL:         DefaultPermitTTL,
		TerminalRetention: DefaultTerminalRetention,
		MaxPending:        DefaultMaxPending,
		MaxTerminal:       DefaultMaxTerminal,
		MaxSafeSummary:    DefaultMaxSafeSummary,
	}
}

type requestKey struct {
	OwnerID, ClientID, TokenFamilyID, WorkspaceID, ManagedWorkspaceID, SessionID string
	Capability                                                                   capability.Capability
	Tool, Fingerprint                                                            string
}

type requestRecord struct {
	input     RequestInput
	requestID string
	status    Status
	version   int
	createdAt time.Time
	expiresAt time.Time
	decidedAt time.Time
}

type terminalRecord struct {
	snapshot Snapshot
	retainAt time.Time
}

type Manager struct {
	mu          sync.Mutex
	now         func() time.Time
	config      Config
	pending     map[string]requestRecord
	pendingKeys map[requestKey]string
	terminal    map[string]terminalRecord
	permits     map[string]*Permit
	closed      bool
}

func New() *Manager {
	manager, _ := NewWithConfig(DefaultConfig())
	return manager
}

func NewWithConfig(config Config) (*Manager, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	return &Manager{
		now:         time.Now,
		config:      config,
		pending:     make(map[string]requestRecord),
		pendingKeys: make(map[requestKey]string),
		terminal:    make(map[string]terminalRecord),
		permits:     make(map[string]*Permit),
	}, nil
}

func validateConfig(config Config) error {
	if config.RequestTTL <= 0 || config.PermitTTL <= 0 || config.TerminalRetention <= 0 ||
		config.MaxPending <= 0 || config.MaxTerminal <= 0 || config.MaxSafeSummary <= 0 {
		return ErrInvalidConfig
	}
	return nil
}

// Create cria ou deduplica um pedido pendente. O segundo retorno indica que o
// snapshot foi reutilizado, não que uma operação foi autorizada.
func (m *Manager) Create(input RequestInput) (Snapshot, bool, error) {
	if len(input.AllowedDecisions) == 0 {
		input.AllowedDecisions = defaultDecisions()
	} else {
		input.AllowedDecisions = append([]Decision(nil), input.AllowedDecisions...)
	}
	if err := validateRequestInput(input, m.config.MaxSafeSummary); err != nil {
		return Snapshot{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Snapshot{}, false, ErrClosed
	}
	now := m.now()
	m.expireLocked(now)
	key := makeRequestKey(input)
	if id, ok := m.pendingKeys[key]; ok {
		return m.snapshotLocked(m.pending[id]), true, nil
	}
	if len(m.pending) >= m.config.MaxPending {
		return Snapshot{}, false, ErrCapacityExceeded
	}
	id, err := randomID("apr")
	if err != nil {
		return Snapshot{}, false, ErrClosed
	}
	record := requestRecord{
		input: input, requestID: id, status: StatusPending, version: 1,
		createdAt: now, expiresAt: now.Add(m.config.RequestTTL),
	}
	m.pending[id] = record
	m.pendingKeys[key] = id
	return m.snapshotLocked(record), false, nil
}

func (m *Manager) ListSnapshots() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.expireLocked(m.now())
	items := make([]Snapshot, 0, len(m.pending)+len(m.terminal))
	for _, record := range m.pending {
		items = append(items, m.snapshotLocked(record))
	}
	for _, record := range m.terminal {
		items = append(items, record.snapshot)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].RequestID < items[j].RequestID
	})
	return items
}

func (m *Manager) GetSnapshot(id string) (Snapshot, error) {
	if strings.TrimSpace(id) != id || id == "" {
		return Snapshot{}, ErrNotFound
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Snapshot{}, ErrClosed
	}
	m.expireLocked(m.now())
	if record, ok := m.pending[id]; ok {
		return m.snapshotLocked(record), nil
	}
	if record, ok := m.terminal[id]; ok {
		return record.snapshot, nil
	}
	return Snapshot{}, ErrNotFound
}

// Decide realiza a transição CAS lógica e, em ALLOW_ONCE, cria um permit
// separado. A rota administrativa usa DecideSnapshot para não expor o permit.
func (m *Manager) Decide(id string, expectedVersion int, decision Decision) (DecisionResult, error) {
	if expectedVersion < 1 || !validDecision(decision) {
		return DecisionResult{}, ErrInvalidDecision
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return DecisionResult{}, ErrClosed
	}
	now := m.now()
	m.expireLocked(now)
	record, ok := m.pending[id]
	if !ok {
		if terminal, exists := m.terminal[id]; exists {
			if terminal.snapshot.Status == StatusExpired {
				return DecisionResult{}, ErrExpired
			}
			return DecisionResult{}, ErrAlreadyDecided
		}
		return DecisionResult{}, ErrNotFound
	}
	if record.version != expectedVersion {
		return DecisionResult{}, ErrStaleVersion
	}
	if record.status != StatusPending {
		return DecisionResult{}, ErrAlreadyDecided
	}
	if !containsDecision(record.input.AllowedDecisions, decision) {
		return DecisionResult{}, ErrInvalidDecision
	}
	if m.config.BeforeDecision != nil {
		if err := m.config.BeforeDecision(m.snapshotLocked(record), decision); err != nil {
			return DecisionResult{}, err
		}
	}
	var permit *Permit
	if decision == DecisionAllowOnce {
		permitID, err := randomID("prm")
		if err != nil {
			return DecisionResult{}, ErrClosed
		}
		permit = &Permit{
			PermitID: permitID, RequestID: record.requestID,
			OwnerID: record.input.OwnerID, ClientID: record.input.ClientID,
			TokenFamilyID: record.input.TokenFamilyID, WorkspaceID: record.input.WorkspaceID,
			SessionID: record.input.SessionID, Capability: record.input.Capability,
			Tool: record.input.Tool, OperationFingerprint: record.input.OperationFingerprint,
			CreatedAt: now, ExpiresAt: now.Add(m.config.PermitTTL),
		}
	}
	delete(m.pending, id)
	delete(m.pendingKeys, makeRequestKey(record.input))
	record.status = StatusApproved
	if decision == DecisionDeny {
		record.status = StatusDenied
	}
	record.version++
	record.decidedAt = now
	snapshot := m.snapshotLocked(record)
	m.terminal[id] = terminalRecord{snapshot: snapshot, retainAt: now.Add(m.config.TerminalRetention)}
	m.pruneTerminalLocked(now)
	if permit != nil {
		m.permits[permit.PermitID] = permit
	}
	return DecisionResult{Request: snapshot, Permit: clonePermit(permit)}, nil
}

func (m *Manager) DecideSnapshot(id string, expectedVersion int, decision Decision) (Snapshot, error) {
	result, err := m.Decide(id, expectedVersion, decision)
	if err != nil {
		return Snapshot{}, err
	}
	return result.Request, nil
}

// Consume revalida toda a autoridade no instante da mutação. GrantActive é
// deliberadamente fornecido pelo chamador owner-side; o permit nunca revive
// uma sessão ou grant revogado.
func (m *Manager) Consume(id string, context ConsumeContext) (Permit, error) {
	if err := validateConsumeContext(context); err != nil {
		return Permit{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Permit{}, ErrClosed
	}
	return m.consumeLocked(id, context, m.now())
}

// ConsumeMatching consome o único permit não consumido que corresponde ao
// contexto completo. O cliente futuro não precisa receber um permit_id; a
// camada de autorização já deve ter revalidado o grant/envelope antes de
// chamar esta porta. Contexto ambíguo falha fechado.
func (m *Manager) ConsumeMatching(context ConsumeContext) (Permit, error) {
	if err := validateConsumeContext(context); err != nil {
		return Permit{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return Permit{}, ErrClosed
	}
	now := m.now()
	m.expireLocked(now)
	matchingID := ""
	for id, permit := range m.permits {
		if !sameContext(permit, context) {
			continue
		}
		if matchingID != "" {
			return Permit{}, ErrPermitAmbiguous
		}
		matchingID = id
	}
	if matchingID == "" {
		return Permit{}, ErrPermitNotFound
	}
	return m.consumeLocked(matchingID, context, now)
}

func (m *Manager) consumeLocked(id string, context ConsumeContext, now time.Time) (Permit, error) {
	permit, ok := m.permits[id]
	if !ok {
		return Permit{}, ErrPermitNotFound
	}
	if !now.Before(permit.ExpiresAt) {
		delete(m.permits, id)
		return Permit{}, ErrPermitExpired
	}
	if permit.ConsumedAt != nil {
		return Permit{}, ErrPermitConsumed
	}
	if !context.GrantActive {
		return Permit{}, ErrPermitRevoked
	}
	if !sameContext(permit, context) {
		return Permit{}, ErrPermitContext
	}
	consumed := now
	permit.ConsumedAt = &consumed
	return *clonePermit(permit), nil
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	clear(m.pending)
	clear(m.pendingKeys)
	clear(m.terminal)
	clear(m.permits)
}

func (m *Manager) expireLocked(now time.Time) {
	for id, record := range m.pending {
		if now.Before(record.expiresAt) {
			continue
		}
		delete(m.pending, id)
		delete(m.pendingKeys, makeRequestKey(record.input))
		record.status = StatusExpired
		record.version++
		snapshot := m.snapshotLocked(record)
		m.terminal[id] = terminalRecord{snapshot: snapshot, retainAt: now.Add(m.config.TerminalRetention)}
	}
	m.pruneTerminalLocked(now)
	for id, permit := range m.permits {
		if !now.Before(permit.ExpiresAt) {
			delete(m.permits, id)
		}
	}
}

func (m *Manager) pruneTerminalLocked(now time.Time) {
	for id, record := range m.terminal {
		if !now.Before(record.retainAt) {
			delete(m.terminal, id)
		}
	}
	if len(m.terminal) <= m.config.MaxTerminal {
		return
	}
	ids := make([]string, 0, len(m.terminal))
	for id := range m.terminal {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := m.terminal[ids[i]].retainAt, m.terminal[ids[j]].retainAt
		if !a.Equal(b) {
			return a.Before(b)
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids[:len(ids)-m.config.MaxTerminal] {
		delete(m.terminal, id)
	}
}

func (m *Manager) snapshotLocked(record requestRecord) Snapshot {
	snapshot := Snapshot{
		RequestID: record.requestID, OwnerID: record.input.OwnerID, ClientID: record.input.ClientID,
		TokenFamilyID: record.input.TokenFamilyID, WorkspaceID: record.input.WorkspaceID,
		ManagedWorkspaceID: record.input.ManagedWorkspaceID,
		SessionID:          record.input.SessionID, Capability: record.input.Capability, Tool: record.input.Tool,
		OperationFingerprint: record.input.OperationFingerprint, SafeSummary: record.input.SafeSummary,
		Status: record.status, Version: record.version, CreatedAt: record.createdAt.UTC(), ExpiresAt: record.expiresAt.UTC(),
	}
	snapshot.AllowedDecisions = append([]Decision(nil), record.input.AllowedDecisions...)
	if !record.decidedAt.IsZero() {
		decided := record.decidedAt.UTC()
		snapshot.DecidedAt = &decided
	}
	return snapshot
}

func makeRequestKey(input RequestInput) requestKey {
	return requestKey{OwnerID: input.OwnerID, ClientID: input.ClientID, TokenFamilyID: input.TokenFamilyID,
		WorkspaceID: input.WorkspaceID, ManagedWorkspaceID: input.ManagedWorkspaceID, SessionID: input.SessionID, Capability: input.Capability,
		Tool: input.Tool, Fingerprint: input.OperationFingerprint}
}

func validateRequestInput(input RequestInput, maxSummary int) error {
	for _, value := range []string{input.OwnerID, input.ClientID, input.WorkspaceID, input.SessionID} {
		if !validIdentifier(value, maximumIdentifierLength) {
			return ErrInvalidInput
		}
	}
	if input.ManagedWorkspaceID != "" && !validIdentifier(input.ManagedWorkspaceID, maximumIdentifierLength) {
		return ErrInvalidInput
	}
	if input.TokenFamilyID != "" && !validIdentifier(input.TokenFamilyID, maximumIdentifierLength) {
		return ErrInvalidInput
	}
	if !capability.IsKnown(input.Capability) {
		return ErrInvalidCapability
	}
	if !validIdentifier(input.Tool, maximumToolLength) {
		return ErrInvalidInput
	}
	if !validFingerprint(input.OperationFingerprint) {
		return ErrInvalidFingerprint
	}
	if !validSummary(input.SafeSummary, maxSummary) {
		return ErrInvalidSummary
	}
	if len(input.AllowedDecisions) == 0 {
		input.AllowedDecisions = defaultDecisions()
	}
	seen := make(map[Decision]struct{}, len(input.AllowedDecisions))
	for _, decision := range input.AllowedDecisions {
		if !validDecision(decision) {
			return ErrInvalidDecision
		}
		if _, ok := seen[decision]; ok {
			return ErrInvalidDecision
		}
		seen[decision] = struct{}{}
	}
	return nil
}

func defaultDecisions() []Decision { return []Decision{DecisionAllowOnce, DecisionDeny} }

func validDecision(decision Decision) bool {
	return decision == DecisionAllowOnce || decision == DecisionAllowSession || decision == DecisionAllowWorkspace || decision == DecisionDeny
}

func containsDecision(decisions []Decision, target Decision) bool {
	for _, decision := range decisions {
		if decision == target {
			return true
		}
	}
	return false
}

func validateConsumeContext(context ConsumeContext) error {
	input := RequestInput{
		OwnerID: context.OwnerID, ClientID: context.ClientID, TokenFamilyID: context.TokenFamilyID,
		WorkspaceID: context.WorkspaceID, SessionID: context.SessionID, Capability: context.Capability,
		Tool: context.Tool, OperationFingerprint: context.OperationFingerprint, SafeSummary: "consume",
	}
	return validateRequestInput(input, len(input.SafeSummary))
}

func validIdentifier(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validSummary(value string, max int) bool {
	if value == "" || len(value) > max || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validFingerprint(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func sameContext(permit *Permit, context ConsumeContext) bool {
	return permit.OwnerID == context.OwnerID && permit.ClientID == context.ClientID &&
		permit.TokenFamilyID == context.TokenFamilyID && permit.WorkspaceID == context.WorkspaceID &&
		permit.SessionID == context.SessionID && permit.Capability == context.Capability &&
		permit.Tool == context.Tool && permit.OperationFingerprint == context.OperationFingerprint
}

func randomID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func clonePermit(permit *Permit) *Permit {
	if permit == nil {
		return nil
	}
	clone := *permit
	if permit.ConsumedAt != nil {
		consumed := *permit.ConsumedAt
		clone.ConsumedAt = &consumed
	}
	return &clone
}
