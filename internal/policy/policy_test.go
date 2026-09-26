package policy

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

func testContext() Context {
	return Context{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family",
		WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite,
		Tool: "write_text_file", Fingerprint: "sha256:fingerprint",
		GrantActive: true, GrantedCapabilities: []capability.Capability{capability.WorkspaceWrite},
	}
}

func TestEngineFailsClosedAndMapsAsk(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	engine := NewEngineWithClock(func() time.Time { return now })
	ctx := testContext()
	if got := engine.Evaluate(ctx); got != RequireApproval {
		t.Fatalf("no rule decision = %q, want %q", got, RequireApproval)
	}
	if got := engine.Evaluate(Context{}); got != Deny {
		t.Fatalf("invalid context decision = %q, want %q", got, Deny)
	}
	if err := engine.SetRule(Rule{OwnerID: "owner", Capability: capability.WorkspaceWrite, Effect: EffectAsk}); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != RequireApproval {
		t.Fatalf("ASK decision = %q, want %q", got, RequireApproval)
	}
}

func TestZeroValueEngineFailsClosedWithoutPanicking(t *testing.T) {
	var engine Engine
	if got := engine.Evaluate(testContext()); got != Deny {
		t.Fatalf("zero-value engine decision = %q, want %q", got, Deny)
	}
}

func TestEngineDeniesMissingCapabilityAndRevokedGrant(t *testing.T) {
	engine := NewEngine()
	ctx := testContext()
	ctx.GrantedCapabilities = nil
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("missing capability decision = %q, want %q", got, Deny)
	}
	if err := engine.SetRule(Rule{OwnerID: "owner", Capability: capability.WorkspaceWrite, Effect: EffectAsk}); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("missing capability with ASK decision = %q, want %q", got, Deny)
	}
	ctx.GrantActive = false
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("revoked grant decision = %q, want %q", got, Deny)
	}
}

func TestEngineSpecificityDenyPrecedenceAndExpiry(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	engine := NewEngineWithClock(func() time.Time { return now })
	ctx := testContext()
	if err := engine.SetRule(Rule{OwnerID: "owner", Capability: capability.WorkspaceWrite, Effect: EffectAllowWorkspace, WorkspaceID: "workspace"}); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != Allow {
		t.Fatalf("workspace allow decision = %q, want %q", got, Allow)
	}
	if err := engine.SetRule(Rule{OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite, Effect: EffectDeny}); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("specific deny decision = %q, want %q", got, Deny)
	}
	expires := now.Add(-time.Second)
	if err := engine.SetRule(Rule{OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite, Tool: "write_text_file", Effect: EffectAsk, ExpiresAt: expires}); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("expired rule changed decision to %q", got)
	}
}

func TestEngineSupportsSessionAndRejectsUnsupportedRules(t *testing.T) {
	engine := NewEngine()
	ctx := testContext()
	if err := engine.SetRule(Rule{OwnerID: "owner", WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite, Effect: EffectAllowSession}); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != Allow {
		t.Fatalf("session allow decision = %q, want %q", got, Allow)
	}
	for _, rule := range []Rule{
		{OwnerID: "owner", Capability: capability.Capability("unknown"), Effect: EffectDeny},
		{OwnerID: "owner", Capability: capability.WorkspaceWrite, Effect: EffectAllowSession},
		{OwnerID: "owner", Capability: capability.WorkspaceWrite, Effect: EffectAllowWorkspace, WorkspaceID: "workspace", SessionID: "session"},
	} {
		if err := engine.SetRule(rule); !errors.Is(err, ErrInvalidPolicyRule) {
			t.Errorf("invalid rule error = %v, want %v", err, ErrInvalidPolicyRule)
		}
	}
	if err := engine.SetRule(Rule{OwnerID: "owner", Capability: capability.WorkspaceWrite, Effect: Effect("ALLOW_ONCE")}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatalf("ALLOW_ONCE error = %v, want %v", err, ErrUnsupportedEffect)
	}
}

func TestEngineCreatesApprovalOnlyForRequireApproval(t *testing.T) {
	engine := NewEngine()
	manager := approval.New()
	defer manager.Close()
	ctx := testContext()
	ctx.Fingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	decision, snapshot, reused, err := engine.EvaluateAndRequest(ctx, manager, "Write src/main.go")
	if err != nil || decision != RequireApproval || reused || snapshot.Status != approval.StatusPending {
		t.Fatalf("approval bridge result = decision=%q snapshot=%+v reused=%t err=%v", decision, snapshot, reused, err)
	}
	decision, duplicate, reused, err := engine.EvaluateAndRequest(ctx, manager, "Write src/main.go")
	if err != nil || decision != RequireApproval || !reused || duplicate.RequestID != snapshot.RequestID {
		t.Fatalf("approval bridge dedup = decision=%q snapshot=%+v reused=%t err=%v", decision, duplicate, reused, err)
	}
	if err := engine.SetRule(Rule{OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite, Effect: EffectDeny}); err != nil {
		t.Fatal(err)
	}
	decision, empty, reused, err := engine.EvaluateAndRequest(ctx, manager, "Write src/main.go")
	if err != nil || decision != Deny || reused || empty.RequestID != "" {
		t.Fatalf("deny created approval: decision=%q snapshot=%+v reused=%t err=%v", decision, empty, reused, err)
	}
}

func TestEngineGrantEnvelopeIsHardCeilingForAllowRules(t *testing.T) {
	engine := NewEngine()
	ctx := testContext()
	ctx.GrantedCapabilities = nil
	for _, effect := range []Effect{EffectAsk, EffectAllowSession, EffectAllowWorkspace} {
		rule := Rule{OwnerID: "owner", WorkspaceID: "workspace", Capability: capability.WorkspaceWrite, Effect: effect}
		if effect == EffectAllowSession {
			rule.SessionID = "session"
		}
		if err := engine.SetRule(rule); err != nil {
			t.Fatal(err)
		}
		if got := engine.Evaluate(ctx); got != Deny {
			t.Fatalf("effect %s escaped grant envelope: %q", effect, got)
		}
	}
	engine = NewEngine()
	ctx.GrantedCapabilities = []capability.Capability{capability.WorkspaceWrite}
	if got := engine.Evaluate(ctx); got != RequireApproval {
		t.Fatalf("capability in envelope without rule = %q, want %q", got, RequireApproval)
	}
}

func TestEngineAppliesSessionAndWorkspaceApprovalsWithRevalidation(t *testing.T) {
	managed := map[string]bool{"managed": true}
	store, err := OpenStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngineWithStore(time.Now, store, func(id string) bool { return managed[id] })
	if err != nil {
		t.Fatal(err)
	}
	ctx := testContext()
	ctx.ManagedWorkspaceID = "managed"
	if err := engine.ApplyApproval(approval.Snapshot{OwnerID: "owner", ClientID: "client", WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite}, approval.DecisionAllowSession); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != Allow {
		t.Fatalf("session policy decision = %q", got)
	}
	if err := engine.ApplyApproval(approval.Snapshot{OwnerID: "owner", ClientID: "client", ManagedWorkspaceID: "managed", Capability: capability.WorkspaceWrite}, approval.DecisionAllowWorkspace); err != nil {
		t.Fatal(err)
	}
	items, err := engine.ListPolicies()
	if err != nil || len(items) != 1 {
		t.Fatalf("persisted policies = %+v err=%v", items, err)
	}
	ctx.WorkspaceID = "managed"
	if got := engine.Evaluate(ctx); got != Allow {
		t.Fatalf("workspace policy decision = %q", got)
	}
	managed["managed"] = false
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("stale workspace decision = %q, want %q", got, Deny)
	}
	managed["managed"] = true
	if err := engine.RevokePolicy(items[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := engine.Evaluate(ctx); got != RequireApproval {
		t.Fatalf("revoked workspace decision = %q", got)
	}
}

func TestEnginePolicyRevokeAndEvaluateAreSafeConcurrently(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngineWithStore(time.Now, store, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyApproval(approval.Snapshot{OwnerID: "owner", ClientID: "client", ManagedWorkspaceID: "managed", Capability: capability.WorkspaceWrite}, approval.DecisionAllowWorkspace); err != nil {
		t.Fatal(err)
	}
	items, err := engine.ListPolicies()
	if err != nil || len(items) != 1 {
		t.Fatal(err)
	}
	ctx := testContext()
	ctx.WorkspaceID = "managed"
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for i := 0; i < 20; i++ {
			_ = engine.Evaluate(ctx)
		}
	}()
	go func() { defer group.Done(); _ = engine.RevokePolicy(items[0].ID) }()
	group.Wait()
	if got := engine.Evaluate(ctx); got != RequireApproval {
		t.Fatalf("post-revoke decision = %q", got)
	}
}

func TestStandardProgrammingProfileUsesClosedEnvelopesAndSessionRules(t *testing.T) {
	engine := NewEngine()
	checkout := StandardProfileInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "session-1", SessionID: "session-1",
		Mode: ProgrammingProfileCheckout,
		GrantedCapabilities: []capability.Capability{
			capability.WorkspaceRead, capability.WorkspaceWrite, capability.WorkspaceDelete, capability.GitReview,
		},
	}
	if err := engine.ApplyStandardProgrammingProfile(checkout); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name       string
		capability capability.Capability
		want       Decision
	}{
		{"read", capability.WorkspaceRead, Allow},
		{"write", capability.WorkspaceWrite, Allow},
		{"review", capability.GitReview, Allow},
		{"delete", capability.WorkspaceDelete, RequireApproval},
	} {
		t.Run(item.name, func(t *testing.T) {
			ctx := testContext()
			ctx.ClientID, ctx.WorkspaceID, ctx.SessionID = "client", "session-1", "session-1"
			ctx.Capability, ctx.GrantedCapabilities = item.capability, checkout.GrantedCapabilities
			if got := engine.Evaluate(ctx); got != item.want {
				t.Fatalf("checkout %s decision = %q, want %q", item.name, got, item.want)
			}
		})
	}
	managed := StandardProfileInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "managed-1", ManagedWorkspaceID: "managed-1", SessionID: "session-2",
		Mode: ProgrammingProfileManaged,
		GrantedCapabilities: []capability.Capability{
			capability.WorkspaceRead, capability.WorkspaceWrite, capability.WorkspaceDelete, capability.GitReview, capability.GitIndex, capability.GitCommit,
		},
	}
	if err := engine.ApplyStandardProgrammingProfile(managed); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name       string
		capability capability.Capability
		want       Decision
	}{
		{"read", capability.WorkspaceRead, Allow},
		{"write", capability.WorkspaceWrite, Allow},
		{"review", capability.GitReview, Allow},
		{"index", capability.GitIndex, Allow},
		{"delete", capability.WorkspaceDelete, RequireApproval},
		{"commit", capability.GitCommit, RequireApproval},
	} {
		t.Run("managed/"+item.name, func(t *testing.T) {
			ctx := testContext()
			ctx.ClientID, ctx.WorkspaceID, ctx.SessionID = "client", "managed-1", "session-2"
			ctx.Capability, ctx.GrantedCapabilities = item.capability, managed.GrantedCapabilities
			if got := engine.Evaluate(ctx); got != item.want {
				t.Fatalf("managed %s decision = %q, want %q", item.name, got, item.want)
			}
		})
	}
	ctx := testContext()
	ctx.ClientID, ctx.WorkspaceID, ctx.SessionID = "client", "managed-1", "session-3"
	ctx.Capability, ctx.GrantedCapabilities = capability.GitIndex, managed.GrantedCapabilities
	if got := engine.Evaluate(ctx); got != RequireApproval {
		t.Fatalf("new session inherited ALLOW_SESSION = %q, want %q", got, RequireApproval)
	}
}

func TestStandardProgrammingProfileRejectsNonCanonicalEnvelope(t *testing.T) {
	engine := NewEngine()
	input := StandardProfileInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "session", SessionID: "session", Mode: ProgrammingProfileCheckout,
		GrantedCapabilities: []capability.Capability{capability.WorkspaceRead, capability.WorkspaceWrite},
	}
	if err := engine.ApplyStandardProgrammingProfile(input); !errors.Is(err, ErrInvalidStandardProfile) {
		t.Fatalf("partial envelope error = %v, want %v", err, ErrInvalidStandardProfile)
	}
}

func TestPersistentWorkspaceAllowOverridesStandardAskWithoutExpandingGrant(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "policies.json"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngineWithStore(time.Now, store, func(id string) bool { return id == "managed" })
	if err != nil {
		t.Fatal(err)
	}
	managed := StandardProfileInput{
		OwnerID: "owner", ClientID: "client", WorkspaceID: "managed", ManagedWorkspaceID: "managed", SessionID: "session",
		Mode: ProgrammingProfileManaged,
		GrantedCapabilities: []capability.Capability{
			capability.WorkspaceRead, capability.WorkspaceWrite, capability.WorkspaceDelete, capability.GitReview, capability.GitIndex, capability.GitCommit,
		},
	}
	if err := engine.ApplyStandardProgrammingProfile(managed); err != nil {
		t.Fatal(err)
	}
	if err := engine.ApplyApproval(approval.Snapshot{OwnerID: "owner", ClientID: "client", ManagedWorkspaceID: "managed", Capability: capability.GitCommit}, approval.DecisionAllowWorkspace); err != nil {
		t.Fatal(err)
	}
	ctx := testContext()
	ctx.ClientID, ctx.WorkspaceID, ctx.SessionID = "client", "managed", "new-session"
	ctx.Capability, ctx.GrantedCapabilities = capability.GitCommit, managed.GrantedCapabilities
	if got := engine.Evaluate(ctx); got != Allow {
		t.Fatalf("persistent workspace allow = %q, want %q", got, Allow)
	}
	ctx.GrantedCapabilities = []capability.Capability{capability.WorkspaceRead}
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("persistent policy escaped grant ceiling = %q, want %q", got, Deny)
	}
}
