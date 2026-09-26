package approval

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

var approvalTestNow = time.Unix(1_800_000_000, 0).UTC()

func newApprovalTestManager(t *testing.T, config Config) *Manager {
	t.Helper()
	manager, err := NewWithConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return approvalTestNow }
	t.Cleanup(manager.Close)
	return manager
}

func approvalTestConfig() Config {
	return Config{
		RequestTTL:        time.Minute,
		PermitTTL:         30 * time.Second,
		TerminalRetention: time.Minute,
		MaxPending:        4,
		MaxTerminal:       4,
		MaxSafeSummary:    64,
	}
}

func approvalTestInput(fingerprint string) RequestInput {
	return RequestInput{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family",
		WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite,
		Tool: "write_text_file", OperationFingerprint: fingerprint, SafeSummary: "Write src/main.go",
	}
}

func approvalFingerprint(last byte) string {
	const hexDigits = "0123456789abcdef"
	value := make([]byte, 64)
	for i := range value {
		value[i] = hexDigits[i%len(hexDigits)]
	}
	value[len(value)-1] = last
	return string(value)
}

func TestManagerCreatesDeduplicatesAndBoundsRequests(t *testing.T) {
	manager := newApprovalTestManager(t, approvalTestConfig())
	first, reused, err := manager.Create(approvalTestInput(approvalFingerprint('a')))
	if err != nil || reused || first.Status != StatusPending || first.Version != 1 {
		t.Fatalf("first request = %+v reused=%t err=%v", first, reused, err)
	}
	duplicate, reused, err := manager.Create(approvalTestInput(approvalFingerprint('a')))
	if err != nil || !reused || duplicate.RequestID != first.RequestID {
		t.Fatalf("duplicate request = %+v reused=%t err=%v", duplicate, reused, err)
	}
	changed, reused, err := manager.Create(approvalTestInput(approvalFingerprint('b')))
	if err != nil || reused || changed.RequestID == first.RequestID {
		t.Fatalf("changed operation was deduplicated: %+v reused=%t err=%v", changed, reused, err)
	}
	if _, _, err := manager.Create(RequestInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session",
		Capability: capability.Capability("unknown"), Tool: "write_text_file",
		OperationFingerprint: approvalFingerprint('c'), SafeSummary: "Write",
	}); !errors.Is(err, ErrInvalidCapability) {
		t.Fatalf("unknown capability error = %v", err)
	}
	if _, _, err := manager.Create(RequestInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session",
		Capability: capability.WorkspaceWrite, Tool: "write_text_file",
		OperationFingerprint: "not-a-fingerprint", SafeSummary: "Write",
	}); !errors.Is(err, ErrInvalidFingerprint) {
		t.Fatalf("invalid fingerprint error = %v", err)
	}
	if _, _, err := manager.Create(RequestInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session",
		Capability: capability.WorkspaceWrite, Tool: "write_text_file",
		OperationFingerprint: approvalFingerprint('d'), SafeSummary: "too long summary" + "012345678901234567890123456789012345678901234567890123456789",
	}); !errors.Is(err, ErrInvalidSummary) {
		t.Fatalf("summary bound error = %v", err)
	}
}

func TestManagerLimitExpiryAndTerminalRetention(t *testing.T) {
	config := approvalTestConfig()
	config.MaxPending = 1
	config.MaxTerminal = 1
	config.TerminalRetention = 10 * time.Second
	manager := newApprovalTestManager(t, config)
	first, _, err := manager.Create(approvalTestInput(approvalFingerprint('a')))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Create(approvalTestInput(approvalFingerprint('b'))); !errors.Is(err, ErrCapacityExceeded) {
		t.Fatalf("pending capacity error = %v", err)
	}
	approvalTestNow = approvalTestNow.Add(2 * time.Minute)
	expired, err := manager.GetSnapshot(first.RequestID)
	if err != nil || expired.Status != StatusExpired || expired.Version != 2 {
		t.Fatalf("expired snapshot = %+v err=%v", expired, err)
	}
	if _, err := manager.DecideSnapshot(first.RequestID, 2, DecisionAllowOnce); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired decision error = %v", err)
	}
	approvalTestNow = approvalTestNow.Add(11 * time.Second)
	if _, err := manager.GetSnapshot(first.RequestID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired terminal retained after retention: %v", err)
	}
	approvalTestNow = time.Unix(1_800_000_000, 0).UTC()
}

func TestManagerDecisionAndPermitLifecycle(t *testing.T) {
	manager := newApprovalTestManager(t, approvalTestConfig())
	request, _, err := manager.Create(approvalTestInput(approvalFingerprint('a')))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.DecideSnapshot(request.RequestID, 2, DecisionAllowOnce); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("stale decision error = %v", err)
	}
	result, err := manager.Decide(request.RequestID, 1, DecisionAllowOnce)
	if err != nil || result.Permit == nil || result.Request.Status != StatusApproved || result.Request.Version != 2 {
		t.Fatalf("allow once result = %+v err=%v", result, err)
	}
	if result.Permit.PermitID == request.RequestID || result.Permit.OperationFingerprint != request.OperationFingerprint {
		t.Fatalf("permit identity is not distinct or bound: %+v", result.Permit)
	}
	if _, err := manager.DecideSnapshot(request.RequestID, 1, DecisionDeny); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("terminal decision error = %v", err)
	}
	deniedRequest, _, err := manager.Create(approvalTestInput(approvalFingerprint('b')))
	if err != nil {
		t.Fatal(err)
	}
	denied, err := manager.Decide(deniedRequest.RequestID, 1, DecisionDeny)
	if err != nil || denied.Permit != nil || denied.Request.Status != StatusDenied {
		t.Fatalf("deny result = %+v err=%v", denied, err)
	}
	context := ConsumeContext{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family", WorkspaceID: "workspace",
		SessionID: "session", Capability: capability.WorkspaceWrite, Tool: "write_text_file",
		OperationFingerprint: request.OperationFingerprint, GrantActive: true,
	}
	if _, err := manager.Consume(result.Permit.PermitID, context); err != nil {
		t.Fatalf("exact permit consume failed: %v", err)
	}
	if _, err := manager.Consume(result.Permit.PermitID, context); !errors.Is(err, ErrPermitConsumed) {
		t.Fatalf("permit replay error = %v", err)
	}
}

func TestManagerRejectsPermitContextExpiryAndRestart(t *testing.T) {
	manager := newApprovalTestManager(t, approvalTestConfig())
	request, _, err := manager.Create(approvalTestInput(approvalFingerprint('a')))
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Decide(request.RequestID, 1, DecisionAllowOnce)
	if err != nil {
		t.Fatal(err)
	}
	context := ConsumeContext{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family", WorkspaceID: "workspace",
		SessionID: "different-session", Capability: capability.WorkspaceWrite, Tool: "write_text_file",
		OperationFingerprint: request.OperationFingerprint, GrantActive: true,
	}
	if _, err := manager.Consume(result.Permit.PermitID, context); !errors.Is(err, ErrPermitContext) {
		t.Fatalf("session mismatch error = %v", err)
	}
	for name, mutate := range map[string]func(*ConsumeContext){
		"owner":       func(value *ConsumeContext) { value.OwnerID = "other-owner" },
		"client":      func(value *ConsumeContext) { value.ClientID = "other-client" },
		"family":      func(value *ConsumeContext) { value.TokenFamilyID = "other-family" },
		"workspace":   func(value *ConsumeContext) { value.WorkspaceID = "other-workspace" },
		"capability":  func(value *ConsumeContext) { value.Capability = capability.GitReview },
		"tool":        func(value *ConsumeContext) { value.Tool = "read_file" },
		"fingerprint": func(value *ConsumeContext) { value.OperationFingerprint = approvalFingerprint('c') },
	} {
		candidate := context
		mutate(&candidate)
		if _, err := manager.Consume(result.Permit.PermitID, candidate); !errors.Is(err, ErrPermitContext) {
			t.Fatalf("%s mismatch error = %v", name, err)
		}
	}
	context.SessionID = "session"
	context.GrantActive = false
	if _, err := manager.Consume(result.Permit.PermitID, context); !errors.Is(err, ErrPermitRevoked) {
		t.Fatalf("revoked grant error = %v", err)
	}
	context.GrantActive = true
	approvalTestNow = approvalTestNow.Add(31 * time.Second)
	if _, err := manager.Consume(result.Permit.PermitID, context); !errors.Is(err, ErrPermitExpired) {
		t.Fatalf("expired permit error = %v", err)
	}

	request, _, err = manager.Create(approvalTestInput(approvalFingerprint('b')))
	if err != nil {
		t.Fatal(err)
	}
	result, err = manager.Decide(request.RequestID, 1, DecisionAllowOnce)
	if err != nil {
		t.Fatal(err)
	}
	manager.Close()
	restarted := New()
	defer restarted.Close()
	if _, err := restarted.GetSnapshot(request.RequestID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("request crossed restart: %v", err)
	}
	if _, err := restarted.Consume(result.Permit.PermitID, context); !errors.Is(err, ErrPermitNotFound) {
		t.Fatalf("permit crossed restart: %v", err)
	}
	approvalTestNow = time.Unix(1_800_000_000, 0).UTC()
}

func TestManagerConcurrentDecisionAndConsumptionAreSingleWinner(t *testing.T) {
	manager := newApprovalTestManager(t, approvalTestConfig())
	request, _, err := manager.Create(approvalTestInput(approvalFingerprint('a')))
	if err != nil {
		t.Fatal(err)
	}
	var decisionWG sync.WaitGroup
	decisionWG.Add(2)
	results := make(chan DecisionResult, 2)
	errorsCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(decision Decision) {
			defer decisionWG.Done()
			result, err := manager.Decide(request.RequestID, 1, decision)
			results <- result
			errorsCh <- err
		}(DecisionAllowOnce)
	}
	decisionWG.Wait()
	close(results)
	close(errorsCh)
	var winner DecisionResult
	winners := 0
	for result := range results {
		if result.Permit != nil {
			winner = result
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("decision winners = %d, want 1", winners)
	}
	for err := range errorsCh {
		if err != nil && !errors.Is(err, ErrAlreadyDecided) {
			t.Fatalf("unexpected concurrent decision error: %v", err)
		}
	}
	context := ConsumeContext{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family", WorkspaceID: "workspace",
		SessionID: "session", Capability: capability.WorkspaceWrite, Tool: "write_text_file",
		OperationFingerprint: approvalFingerprint('a'), GrantActive: true,
	}
	var consumeWG sync.WaitGroup
	consumeWG.Add(2)
	consumeErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			defer consumeWG.Done()
			_, err := manager.Consume(winner.Permit.PermitID, context)
			consumeErrors <- err
		}()
	}
	consumeWG.Wait()
	close(consumeErrors)
	consumed := 0
	for err := range consumeErrors {
		if err == nil {
			consumed++
		} else if !errors.Is(err, ErrPermitConsumed) {
			t.Fatalf("unexpected concurrent consumption error: %v", err)
		}
	}
	if consumed != 1 {
		t.Fatalf("consumption winners = %d, want 1", consumed)
	}
}
