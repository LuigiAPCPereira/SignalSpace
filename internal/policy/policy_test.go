package policy

import (
	"errors"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
)

func testContext() Context {
	return Context{
		OwnerID: "owner", ClientID: "client", TokenFamilyID: "family",
		WorkspaceID: "workspace", SessionID: "session", Capability: capability.WorkspaceWrite,
		Tool: "write_text_file", Fingerprint: "sha256:fingerprint",
	}
}

func TestEngineFailsClosedAndMapsAsk(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	engine := NewEngineWithClock(func() time.Time { return now })
	ctx := testContext()
	if got := engine.Evaluate(ctx); got != Deny {
		t.Fatalf("no rule decision = %q, want %q", got, Deny)
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
