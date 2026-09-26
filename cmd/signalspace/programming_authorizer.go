package main

import (
	"context"
	"errors"

	"github.com/LuigiAPCPereira/SignalSpace/internal/approval"
	"github.com/LuigiAPCPereira/SignalSpace/internal/mcp"
	"github.com/LuigiAPCPereira/SignalSpace/internal/policy"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

// programmingAuthorizer é a única composição concreta da ponte pública.
// OAuth já foi validado pelo transporte; aqui o owner-side revalida o grant,
// o envelope, a policy e o permit antes de delegar efeito às portas typed.
type programmingAuthorizer struct {
	owner     string
	grants    *workspace.Grants
	policies  *policy.Engine
	approvals *approval.Manager
}

func (a *programmingAuthorizer) Authorize(_ context.Context, input mcp.ProgrammingAuthorizationInput) error {
	if a == nil || a.grants == nil || a.policies == nil || a.approvals == nil || input.Identity.OwnerSubject != a.owner || input.Identity.ClientID == "" {
		return programmingError("LOCAL_APPROVAL_UNAVAILABLE", input, "")
	}
	snapshot, err := a.grants.Snapshot()
	if err != nil {
		return programmingError("LOCAL_APPROVAL_UNAVAILABLE", input, "")
	}
	grantActive := snapshot.Active && snapshot.ClientID == input.Identity.ClientID && snapshot.SessionID == input.SessionID
	workspaceID := snapshot.ManagedWorkspaceID
	if workspaceID == "" {
		// Checkout comum não tem um ID de worktree persistente; o session ID é
		// o identificador local estável dessa concessão corrente.
		workspaceID = snapshot.SessionID
	}
	context := policy.Context{
		OwnerID:             input.Identity.OwnerSubject,
		ClientID:            input.Identity.ClientID,
		WorkspaceID:         workspaceID,
		ManagedWorkspaceID:  snapshot.ManagedWorkspaceID,
		SessionID:           snapshot.SessionID,
		Capability:          input.Capability,
		Tool:                input.Tool,
		Fingerprint:         input.Fingerprint,
		GrantActive:         grantActive,
		GrantedCapabilities: snapshot.Capabilities,
	}
	decision := a.policies.Evaluate(context)
	switch decision {
	case policy.Deny:
		return programmingError("LOCAL_AUTHORIZATION_DENIED", input, "")
	case policy.Allow:
		return nil
	case policy.RequireApproval:
		consumeContext := approval.ConsumeContext{
			OwnerID: input.Identity.OwnerSubject, ClientID: input.Identity.ClientID,
			WorkspaceID: workspaceID, SessionID: snapshot.SessionID,
			Capability: input.Capability, Tool: input.Tool,
			OperationFingerprint: input.Fingerprint, GrantActive: grantActive,
		}
		if _, consumeErr := a.approvals.ConsumeMatching(consumeContext); consumeErr == nil {
			return nil
		} else if !errors.Is(consumeErr, approval.ErrPermitNotFound) && !errors.Is(consumeErr, approval.ErrPermitExpired) && !errors.Is(consumeErr, approval.ErrPermitConsumed) {
			return programmingError("LOCAL_APPROVAL_UNAVAILABLE", input, "")
		}
		_, request, _, requestErr := a.policies.EvaluateAndRequest(context, a.approvals, input.SafeSummary)
		if requestErr != nil {
			return programmingError("LOCAL_APPROVAL_UNAVAILABLE", input, "")
		}
		return programmingError("LOCAL_APPROVAL_REQUIRED", input, request.RequestID)
	default:
		return programmingError("LOCAL_APPROVAL_UNAVAILABLE", input, "")
	}
}

func programmingError(code string, input mcp.ProgrammingAuthorizationInput, requestID string) error {
	return &mcp.ProgrammingAuthorizationError{
		Code: code, RequestID: requestID, Capability: input.Capability, Summary: input.SafeSummary,
	}
}
