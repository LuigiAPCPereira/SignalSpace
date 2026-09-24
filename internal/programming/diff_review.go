package programming

import (
	"context"
	"errors"
	"os/exec"
	"strings"
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
	StagedDiff      string
	OutputTruncated bool
}

// DiffReview compara duas observações feitas pela mesma sessão local.
type DiffReview struct {
	Before           GitSnapshot
	After            GitSnapshot
	Complete         bool
	StatusChange     bool
	DiffChange       bool
	StagedDiffChange bool
}

func CaptureGitSnapshot(ctx context.Context, session workspace.ProcessDirectory, outputLimit int) (GitSnapshot, error) {
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
		diff, diffTruncated, readErr := runGitRead(readCtx, dir, outputLimit, "diff", "--no-ext-diff", "--no-textconv", "--binary", "--")
		if readErr != nil {
			return readErr
		}
		stagedDiff, stagedTruncated, readErr := runGitRead(readCtx, dir, outputLimit, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--binary", "--")
		if readErr != nil {
			return readErr
		}
		snapshot = GitSnapshot{Status: status, Diff: diff, StagedDiff: stagedDiff, OutputTruncated: statusTruncated || diffTruncated || stagedTruncated}
		return nil
	})
	return snapshot, err
}

func CompareGitSnapshots(before, after GitSnapshot) DiffReview {
	return DiffReview{
		Before:           before,
		After:            after,
		Complete:         !before.OutputTruncated && !after.OutputTruncated,
		StatusChange:     before.Status != after.Status,
		DiffChange:       before.Diff != after.Diff,
		StagedDiffChange: before.StagedDiff != after.StagedDiff,
	}
}

func runGitRead(ctx context.Context, dir string, outputLimit int, args ...string) (string, bool, error) {
	cmdArgs := []string{
		"--no-optional-locks",
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.preloadIndex=false",
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	cmd.Dir = dir
	cmd.Env = safeGitReadEnvironment(cmd.Environ())
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

// safeGitReadEnvironment remove mecanismos ambientais que podem trocar a
// configuração, o índice, os objetos ou um executor auxiliar da leitura.
// Configuração local legítima continua disponível, mas as chaves executáveis
// usadas por status/diff são neutralizadas nos argumentos acima.
func safeGitReadEnvironment(environment []string) []string {
	blocked := map[string]struct{}{
		"GIT_EXTERNAL_DIFF":                {},
		"GIT_DIFF_OPTS":                    {},
		"GIT_DIR":                          {},
		"GIT_WORK_TREE":                    {},
		"GIT_COMMON_DIR":                   {},
		"GIT_INDEX_FILE":                   {},
		"GIT_OBJECT_DIRECTORY":             {},
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": {},
		"GIT_OBJECT_DIRECTORY_RELATIVE":    {},
		"GIT_QUARANTINE_PATH":              {},
		"GIT_EXEC_PATH":                    {},
		"GIT_ASKPASS":                      {},
		"SSH_ASKPASS":                      {},
		"GIT_SSH":                          {},
		"GIT_SSH_COMMAND":                  {},
		"GIT_PAGER":                        {},
		"GIT_EDITOR":                       {},
		"GIT_SEQUENCE_EDITOR":              {},
	}
	filtered := make([]string, 0, len(environment)+5)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_CONFIG_") || strings.HasPrefix(key, "GIT_ATTR_") {
			continue
		}
		if _, ok := blocked[key]; ok {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
	)
}
