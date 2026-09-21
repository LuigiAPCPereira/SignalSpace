package programming

import (
	"context"
	"errors"
	"os/exec"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

var ErrInvalidDiffReview = errors.New("invalid local diff review")

// GitSnapshot é uma observação limitada e somente leitura do estado Git da
// raiz aprovada. Capturar o snapshot não adiciona, confirma, publica ou altera
// arquivos.
type GitSnapshot struct {
	Status          string
	Diff            string
	OutputTruncated bool
}

// DiffReview compara duas observações feitas pela mesma sessão local.
type DiffReview struct {
	Before       GitSnapshot
	After        GitSnapshot
	StatusChange bool
	DiffChange   bool
}

func CaptureGitSnapshot(ctx context.Context, session *workspace.Session, outputLimit int) (GitSnapshot, error) {
	if ctx == nil || session == nil || outputLimit < 1 || outputLimit > maxOutputLimit {
		return GitSnapshot{}, ErrInvalidDiffReview
	}
	dir, err := session.ProcessDir()
	if err != nil {
		return GitSnapshot{}, err
	}
	status, statusTruncated, err := runGitRead(ctx, dir, outputLimit, "status", "--porcelain=v1", "--untracked-files=all", "--")
	if err != nil {
		return GitSnapshot{}, err
	}
	diff, diffTruncated, err := runGitRead(ctx, dir, outputLimit, "diff", "--no-ext-diff", "--binary", "--")
	if err != nil {
		return GitSnapshot{}, err
	}
	return GitSnapshot{Status: status, Diff: diff, OutputTruncated: statusTruncated || diffTruncated}, nil
}

func CompareGitSnapshots(before, after GitSnapshot) DiffReview {
	return DiffReview{
		Before:       before,
		After:        after,
		StatusChange: before.Status != after.Status,
		DiffChange:   before.Diff != after.Diff,
	}
}

func runGitRead(ctx context.Context, dir string, outputLimit int, args ...string) (string, bool, error) {
	cmdArgs := append([]string(nil), args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	output := &limitedBuffer{limit: outputLimit}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return output.String(), output.truncated, err
	}
	return output.String(), output.truncated, nil
}
