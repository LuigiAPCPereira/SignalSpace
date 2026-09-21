package programming

import (
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

var ErrInvalidDiffReview = errors.New("invalid local diff review")

const defaultGitTimeout = 10 * time.Second

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
	Complete     bool
	StatusChange bool
	DiffChange   bool
}

func CaptureGitSnapshot(ctx context.Context, session *workspace.Session, outputLimit int) (GitSnapshot, error) {
	if ctx == nil || session == nil || outputLimit < 1 || outputLimit > maxOutputLimit {
		return GitSnapshot{}, ErrInvalidDiffReview
	}
	readCtx, cancel := context.WithTimeout(ctx, defaultGitTimeout)
	defer cancel()
	var snapshot GitSnapshot
	err := session.WithProcessDir(func(dir string) error {
		status, statusTruncated, readErr := runGitRead(readCtx, dir, outputLimit, "status", "--porcelain=v1", "--untracked-files=all", "--")
		if readErr != nil {
			return readErr
		}
		diff, diffTruncated, readErr := runGitRead(readCtx, dir, outputLimit, "diff", "--no-ext-diff", "--binary", "--")
		if readErr != nil {
			return readErr
		}
		snapshot = GitSnapshot{Status: status, Diff: diff, OutputTruncated: statusTruncated || diffTruncated}
		return nil
	})
	return snapshot, err
}

func CompareGitSnapshots(before, after GitSnapshot) DiffReview {
	return DiffReview{
		Before:       before,
		After:        after,
		Complete:     !before.OutputTruncated && !after.OutputTruncated,
		StatusChange: before.Status != after.Status,
		DiffChange:   before.Diff != after.Diff,
	}
}

func runGitRead(ctx context.Context, dir string, outputLimit int, args ...string) (string, bool, error) {
	cmdArgs := append([]string(nil), args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &limitedBuffer{limit: outputLimit}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return output.String(), output.truncated, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return output.String(), output.truncated, err
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
		return output.String(), output.truncated, ctx.Err()
	}
}
