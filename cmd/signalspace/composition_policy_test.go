package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/tunnel"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type countedReader struct {
	reads int
}

func (r *countedReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

func TestCompositionPolicyAllowsOnlyDiagnosticAndRead(t *testing.T) {
	tests := []struct {
		name        string
		mode        compositionMode
		readScope   string
		consoleMode workspaceConsoleMode
		validator   compositionValidatorMode
		mcpAddress  string
	}{
		{"diagnostic", compositionDiagnostic, "", workspaceConsoleApprovalsOnly, compositionLocalOAuthJWTValidator, admin.PublicAddress},
		{"read", compositionRead, workspace.ScopeRead, workspaceConsoleRead, compositionLocalOAuthJWTValidator, admin.PublicAddress},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := planComposition(test.mode)
			if err != nil {
				t.Fatal(err)
			}
			if plan.mode != test.mode || plan.oauthScope != compositionDiagnosticScope || plan.workspaceReadScope != test.readScope || plan.consoleMode != test.consoleMode || plan.validatorMode != test.validator || plan.mcpAddress != test.mcpAddress {
				t.Fatalf("unexpected composition plan: %+v", plan)
			}
		})
	}
	for _, mode := range []compositionMode{compositionInvalid, compositionMode(255)} {
		if _, err := planComposition(mode); err == nil {
			t.Fatalf("unsupported composition %d was accepted", mode)
		}
	}
}

func TestEmbeddedHandlerRejectsUnsupportedCompositionBeforeOAuthState(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "must-not-be-created")
	handler, authorization, console, err := embeddedHandlerWithWorkspace(readTestResource, stateDir, compositionMode(255))
	if err == nil || handler != nil || authorization != nil || console != nil {
		t.Fatalf("unsupported composition returned live components: handler=%v auth=%v console=%v err=%v", handler, authorization, console, err)
	}
	if _, statErr := os.Stat(stateDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unsupported composition touched OAuth state path: %v", statErr)
	}

	plan, err := planComposition(compositionDiagnostic)
	if err != nil {
		t.Fatal(err)
	}
	plan.workspaceReadScope = workspace.ScopeWrite
	if handler, authorization, console, err := embeddedHandlerForPlan(readTestResource, stateDir, plan); err == nil || handler != nil || authorization != nil || console != nil {
		t.Fatalf("modified plan bypassed closed policy: handler=%v auth=%v console=%v err=%v", handler, authorization, console, err)
	}
	if _, statErr := os.Stat(stateDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("modified plan touched OAuth state path: %v", statErr)
	}

	plan, err = planComposition(compositionDiagnostic)
	if err != nil {
		t.Fatal(err)
	}
	plan.validatorMode = 0
	if handler, authorization, console, err := embeddedHandlerForPlan(readTestResource, stateDir, plan); err == nil || handler != nil || authorization != nil || console != nil {
		t.Fatalf("plan without a local validator was accepted: handler=%v auth=%v console=%v err=%v", handler, authorization, console, err)
	}
	plan, err = planComposition(compositionDiagnostic)
	if err != nil {
		t.Fatal(err)
	}
	plan.mcpAddress = admin.AdminAddress
	if handler, authorization, console, err := embeddedHandlerForPlan(readTestResource, stateDir, plan); err == nil || handler != nil || authorization != nil || console != nil {
		t.Fatalf("plan exposing the administrative port as MCP was accepted: handler=%v auth=%v console=%v err=%v", handler, authorization, console, err)
	}
	if _, statErr := os.Stat(stateDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid plans touched OAuth state path: %v", statErr)
	}
}

func TestQuickRejectsUnsupportedCompositionBeforeAnySideEffect(t *testing.T) {
	t.Setenv("SIGNALSPACE_AUTH_MODE", "embedded")
	input := &countedReader{}
	var output bytes.Buffer
	started, verified, adminFactoryCalled := false, false, false
	err := runQuickWithAdminFactory(context.Background(), input, &output,
		func(context.Context) (*tunnel.Quick, error) {
			started = true
			return nil, errors.New("tunnel must not start")
		},
		func(context.Context, string) (mcp.TransportReport, error) {
			verified = true
			return mcp.TransportReport{}, errors.New("transport must not be checked")
		},
		compositionMode(255), false,
		func(http.Handler) *http.Server {
			adminFactoryCalled = true
			return &http.Server{}
		},
	)
	if err == nil || !strings.Contains(err.Error(), "allowed modes are diagnostic and read") {
		t.Fatalf("unsupported mode did not fail at the composition boundary: %v", err)
	}
	if input.reads != 0 || output.Len() != 0 || started || verified || adminFactoryCalled {
		t.Fatalf("unsupported mode caused side effects: reads=%d output=%q started=%t verified=%t admin=%t", input.reads, output.String(), started, verified, adminFactoryCalled)
	}
}

func TestQuickAdminPanelDoesNotChangeCompositionMode(t *testing.T) {
	mode, panel, ok := quickModeArgs([]string{"connect", "quick", "read", "panel"})
	if !ok || mode != compositionRead || !panel {
		t.Fatalf("panel altered the read composition: mode=%d panel=%t ok=%t", mode, panel, ok)
	}
	mode, panel, ok = quickModeArgs([]string{"connect", "quick", "panel"})
	if !ok || mode != compositionDiagnostic || !panel {
		t.Fatalf("panel altered the diagnostic composition: mode=%d panel=%t ok=%t", mode, panel, ok)
	}
}
