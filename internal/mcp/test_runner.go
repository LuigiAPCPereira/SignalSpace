package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
)

const (
	testRunScope    = "signalspace:test.run"
	testRunToolName = "run_workspace_tests"
)

// WorkspaceTestRunner é uma porta estrita para a composição MCP de teste.
// A implementação deve revalidar owner, cliente, sessão e concessão atual;
// nenhuma raiz, Session ou comando chega ao transporte.
type WorkspaceTestRunner interface {
	RunTests(context.Context, string, string, string) (programming.TestResult, error)
}

type testToolAccess struct {
	runner    WorkspaceTestRunner
	verify    func(context.Context) (VerifiedIdentity, error)
	challenge string
	advertise bool
}

func testRunToolDefinition() map[string]any {
	return map[string]any{
		"name":        testRunToolName,
		"description": "Run the fixed go test ./... command in the approved workspace. It is not a process sandbox and accepts no command, path or environment arguments.",
		"inputSchema": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"session_id": map[string]any{"type": "string"}},
			"required":             []string{"session_id"},
			"additionalProperties": false,
		},
		"securitySchemes": []any{map[string]any{"type": "oauth2", "scopes": []string{testRunScope}}},
		"annotations":     map[string]any{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false},
	}
}

func (a *testToolAccess) call(w http.ResponseWriter, ctx context.Context, id any, raw json.RawMessage) {
	identity, err := a.verify(ctx)
	if err != nil {
		challenge := a.challenge + `, error="invalid_token", error_description="Invalid access token"`
		if errors.Is(err, ErrInsufficientScope) {
			challenge = a.challenge + `, error="insufficient_scope", error_description="Test execution scope is required"`
		}
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Test execution authorization required."}},
			"_meta":   map[string]any{"mcp/www_authenticate": []string{challenge}},
			"isError": true,
		}})
		return
	}
	var arguments map[string]json.RawMessage
	if json.Unmarshal(raw, &arguments) != nil || len(arguments) != 1 {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	sessionID, ok := requiredString(arguments, "session_id")
	if !ok || sessionID == "" {
		fail(w, http.StatusOK, id, -32602, "Invalid params")
		return
	}
	result, err := a.runner.RunTests(ctx, identity.OwnerSubject, identity.ClientID, sessionID)
	if err != nil {
		reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Test execution unavailable or not authorized."}},
			"isError": true,
		}})
		return
	}
	payload, err := json.Marshal(testRunResult{
		Command:         result.Command,
		ExitCode:        result.ExitCode,
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		OutputTruncated: result.OutputTruncated,
		TimedOut:        result.TimedOut,
		Canceled:        result.Canceled,
		Terminated:      result.Terminated,
		Status:          string(result.Status()),
		TerminationNote: "terminated means the main go process was observed by cmd.Wait; descendant termination is not independently attested",
	})
	if err != nil {
		fail(w, http.StatusOK, id, -32603, "Test execution unavailable")
		return
	}
	reply(w, http.StatusOK, response{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"content": []any{map[string]any{"type": "text", "text": string(payload)}},
		"isError": false,
	}})
}

type testRunResult struct {
	Command         []string `json:"command"`
	ExitCode        int      `json:"exit_code"`
	Stdout          string   `json:"stdout"`
	Stderr          string   `json:"stderr"`
	OutputTruncated bool     `json:"output_truncated"`
	TimedOut        bool     `json:"timed_out"`
	Canceled        bool     `json:"canceled"`
	Terminated      bool     `json:"terminated"`
	Status          string   `json:"status"`
	TerminationNote string   `json:"termination_note"`
}
