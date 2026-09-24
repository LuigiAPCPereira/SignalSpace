package main

import (
	"context"
	"errors"
	"sync"

	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

const programmingGitOutputLimit = 8192

// workspaceGitReviewer mantém um baseline por sessão aprovado localmente.
// A porta MCP recebe apenas identidade e session ID; a raiz continua dentro de
// Grants e cada observação revalida a concessão Git corrente.
type workspaceGitReviewer struct {
	grants      *workspace.Grants
	outputLimit int

	mu        sync.Mutex
	baselines map[string]programming.GitSnapshot
}

func newWorkspaceGitReviewer(grants *workspace.Grants, outputLimit int) *workspaceGitReviewer {
	return &workspaceGitReviewer{
		grants:      grants,
		outputLimit: outputLimit,
		baselines:   make(map[string]programming.GitSnapshot),
	}
}

// CaptureBaseline registra a observação antes de qualquer edição autorizada.
// Se a raiz não for um repositório Git observável, a aprovação é recusada pelo
// chamador e a concessão recém-criada é imediatamente revogada.
func (r *workspaceGitReviewer) CaptureBaseline(owner, clientID, sessionID string) error {
	if r == nil || r.grants == nil {
		return errors.New("Git reviewer unavailable")
	}
	var baseline programming.GitSnapshot
	err := r.grants.WithAuthorizedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		baseline, err = programming.CaptureGitSnapshot(context.Background(), directory, r.outputLimit)
		return err
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.baselines[sessionID] = baseline
	r.mu.Unlock()
	return nil
}

func (r *workspaceGitReviewer) ReviewGit(ctx context.Context, owner, clientID, sessionID string) (programming.DiffReview, error) {
	if r == nil || r.grants == nil || ctx == nil {
		return programming.DiffReview{}, errors.New("Git reviewer unavailable")
	}
	r.mu.Lock()
	baseline, ok := r.baselines[sessionID]
	r.mu.Unlock()
	if !ok {
		return programming.DiffReview{}, workspace.ErrNotAuthorized
	}
	var after programming.GitSnapshot
	err := r.grants.WithAuthorizedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		after, err = programming.CaptureGitSnapshot(ctx, directory, r.outputLimit)
		return err
	})
	if err != nil {
		return programming.DiffReview{}, err
	}
	return programming.CompareGitSnapshots(baseline, after), nil
}

func (r *workspaceGitReviewer) GitStatus(ctx context.Context, owner, clientID, sessionID string) (workspace.GitIndexStatus, error) {
	if r == nil || r.grants == nil || ctx == nil {
		return workspace.GitIndexStatus{}, errors.New("Git status unavailable")
	}
	var status workspace.GitIndexStatus
	err := r.grants.WithAuthorizedGitProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		status, err = workspace.CaptureGitIndexStatus(directory)
		return err
	})
	return status, err
}

func (r *workspaceGitReviewer) Forget(sessionID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.baselines, sessionID)
	r.mu.Unlock()
}

func (r *workspaceGitReviewer) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.baselines = make(map[string]programming.GitSnapshot)
	r.mu.Unlock()
}
