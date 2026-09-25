package main

import (
	"context"
	"errors"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

// workspaceGitCommitter é a única composição entre Grants, identidade privada
// e lifecycle managed. O transporte nunca recebe caminho, ref ou identidade.
type workspaceGitCommitter struct {
	grants  *workspace.Grants
	managed *workspace.ManagedWorktreeManager
}

func newWorkspaceGitCommitter(grants *workspace.Grants, managed *workspace.ManagedWorktreeManager) *workspaceGitCommitter {
	return &workspaceGitCommitter{grants: grants, managed: managed}
}

func (c *workspaceGitCommitter) CommitGitIndex(ctx context.Context, owner, clientID, sessionID string, request workspace.GitCommitRequest) (workspace.GitCommitResult, error) {
	if c == nil || c.grants == nil || c.managed == nil || ctx == nil {
		return workspace.GitCommitResult{Status: "failed_no_ref_change"}, errors.New("Git commit unavailable")
	}
	var result workspace.GitCommitResult
	err := c.grants.WithAuthorizedManagedGitCommit(owner, clientID, sessionID, func(directory workspace.ProcessDirectory, metadata workspace.WorkspaceMetadata) error {
		var err error
		result, err = c.managed.CommitGitIndex(metadata.ManagedWorkspaceID, directory, request)
		return err
	})
	return result, err
}
