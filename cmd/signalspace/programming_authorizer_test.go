package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/capability"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/policy"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func TestProgrammingAuthorizerOneShotApprovalAndExactRetry(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	grants, err := workspace.NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	clientID := strings.Repeat("a", 32)
	sessionID, err := grants.GrantProgrammingCheckout(root, clientID)
	if err != nil {
		t.Fatal(err)
	}
	engine := policy.NewEngine()
	manager := approval.New()
	defer manager.Close()
	bridge := &programmingAuthorizer{owner: "owner", grants: grants, policies: engine, approvals: manager}
	input := mcp.ProgrammingAuthorizationInput{
		Identity: mcp.VerifiedIdentity{OwnerSubject: "owner", ClientID: clientID},
		Tool:     "write_text_file", Capability: capability.WorkspaceWrite, SessionID: sessionID,
		Fingerprint: strings.Repeat("b", 64), Operation: map[string]any{"path": "file.txt"}, SafeSummary: "write_text_file file.txt",
	}
	firstErr := bridge.Authorize(context.Background(), input)
	var firstApproval *mcp.ProgrammingAuthorizationError
	if !errors.As(firstErr, &firstApproval) || firstApproval.Code != "LOCAL_APPROVAL_REQUIRED" || firstApproval.RequestID == "" {
		t.Fatalf("first call = %v, want local approval request", firstErr)
	}
	if _, err := manager.DecideSnapshot(firstApproval.RequestID, 1, approval.DecisionAllowOnce); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Authorize(context.Background(), input); err != nil {
		t.Fatalf("exact retry was not allowed after one-shot approval: %v", err)
	}
	thirdErr := bridge.Authorize(context.Background(), input)
	var thirdApproval *mcp.ProgrammingAuthorizationError
	if !errors.As(thirdErr, &thirdApproval) || thirdApproval.Code != "LOCAL_APPROVAL_REQUIRED" || thirdApproval.RequestID == firstApproval.RequestID {
		t.Fatalf("third retry = %v, want a new approval request", thirdErr)
	}
}

func TestProgrammingAuthorizerDenyDoesNotCreateSecondEffect(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	grants, err := workspace.NewGrants("owner")
	if err != nil {
		t.Fatal(err)
	}
	defer grants.Close()
	clientID := strings.Repeat("c", 32)
	sessionID, err := grants.GrantProgrammingCheckout(root, clientID)
	if err != nil {
		t.Fatal(err)
	}
	engine := policy.NewEngine()
	manager := approval.New()
	defer manager.Close()
	bridge := &programmingAuthorizer{owner: "owner", grants: grants, policies: engine, approvals: manager}
	input := mcp.ProgrammingAuthorizationInput{Identity: mcp.VerifiedIdentity{OwnerSubject: "owner", ClientID: clientID}, Tool: "delete_file", Capability: capability.WorkspaceDelete, SessionID: sessionID, Fingerprint: strings.Repeat("d", 64), SafeSummary: "delete_file tmp.txt"}
	firstErr := bridge.Authorize(context.Background(), input)
	var pending *mcp.ProgrammingAuthorizationError
	if !errors.As(firstErr, &pending) || pending.Code != "LOCAL_APPROVAL_REQUIRED" {
		t.Fatalf("first call = %v", firstErr)
	}
	if _, err := manager.DecideSnapshot(pending.RequestID, 1, approval.DecisionDeny); err != nil {
		t.Fatal(err)
	}
	secondErr := bridge.Authorize(context.Background(), input)
	var secondPending *mcp.ProgrammingAuthorizationError
	if !errors.As(secondErr, &secondPending) || secondPending.Code != "LOCAL_APPROVAL_REQUIRED" {
		t.Fatalf("retry after deny = %v, want a fresh approval request", secondErr)
	}
	if secondPending.RequestID == pending.RequestID {
		t.Fatal("denied request was reused")
	}
}
