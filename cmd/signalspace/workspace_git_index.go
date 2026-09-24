package main

import (
	"context"
	"errors"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

// workspaceGitIndexOperator é a composição local da porta experimental do
// índice. O entrypoint público atual não o injeta; mantê-lo separado impede
// que stage/unstage apareçam na aprovação normal por acidente.
type workspaceGitIndexOperator struct {
	grants *workspace.Grants
}

func newWorkspaceGitIndexOperator(grants *workspace.Grants) *workspaceGitIndexOperator {
	return &workspaceGitIndexOperator{grants: grants}
}

func (o *workspaceGitIndexOperator) StageGitPaths(ctx context.Context, owner, clientID, sessionID, expected string, entries []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error) {
	if o == nil || o.grants == nil || ctx == nil {
		return workspace.GitIndexMutationResult{}, errors.New("Git index unavailable")
	}
	var result workspace.GitIndexMutationResult
	err := o.grants.WithAuthorizedManagedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		result, err = workspace.StageGitPaths(directory, expected, entries)
		return err
	})
	return result, err
}

func (o *workspaceGitIndexOperator) UnstageGitPaths(ctx context.Context, owner, clientID, sessionID, expected string, entries []workspace.GitIndexEntry) (workspace.GitIndexMutationResult, error) {
	if o == nil || o.grants == nil || ctx == nil {
		return workspace.GitIndexMutationResult{}, errors.New("Git index unavailable")
	}
	var result workspace.GitIndexMutationResult
	err := o.grants.WithAuthorizedManagedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		result, err = workspace.UnstageGitPaths(directory, expected, entries)
		return err
	})
	return result, err
}
