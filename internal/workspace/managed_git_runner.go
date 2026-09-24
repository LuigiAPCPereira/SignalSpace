package workspace

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	managedGitTimeout = 30 * time.Second
	managedGitOutput  = 64 << 10
)

var ErrManagedGit = errors.New("managed Git operation failed")

type managedGitError struct {
	exitCode int
	err      error
}

func (e *managedGitError) Error() string { return ErrManagedGit.Error() }
func (e *managedGitError) Unwrap() error { return e.err }

func managedGitExitCode(err error) (int, bool) {
	var commandErr *managedGitError
	if !errors.As(err, &commandErr) {
		return 0, false
	}
	return commandErr.exitCode, true
}

type managedGitRunner struct {
	hooksPath string
}

func (r managedGitRunner) run(directory string, args ...string) error {
	_, err := r.capture(directory, args...)
	return err
}

func (r managedGitRunner) runInput(directory string, input []byte, args ...string) error {
	_, _, err := r.captureInput(directory, input, args...)
	return err
}

func (r managedGitRunner) capture(directory string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), managedGitTimeout)
	defer cancel()

	gitArgs := []string{
		"--no-optional-locks",
		"-c", "core.hooksPath=" + r.hooksPath,
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.preloadIndex=false",
	}
	gitArgs = append(gitArgs, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = directory
	cmd.Env = managedGitEnvironment(cmd.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var output limitedManagedGitBuffer
	output.limit = managedGitOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return output.String(), errors.Join(ErrManagedGit, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return output.String(), nil
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return output.String(), &managedGitError{exitCode: exitCode, err: err}
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
		return output.String(), errors.Join(ErrManagedGit, ctx.Err())
	}
}

// captureOutput mantém stdout separado de stderr para comandos cujo formato
// estruturado é consumido pelo domínio. A configuração e o ambiente são os
// mesmos do runner mutável; nenhum argumento vem do cliente.
func (r managedGitRunner) captureOutput(directory string, limit int, args ...string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), managedGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", r.arguments(args...)...)
	cmd.Dir = directory
	cmd.Env = managedGitEnvironment(cmd.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr limitedManagedGitBuffer
	stdout.limit = limit
	stderr.limit = managedGitOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return stdout.Bytes(), stdout.truncated, errors.Join(ErrManagedGit, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil && !stdout.truncated {
			return stdout.Bytes(), false, nil
		}
		if err == nil {
			return stdout.Bytes(), true, errors.Join(ErrManagedGit, errors.New("managed Git output exceeded limit"))
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return stdout.Bytes(), stdout.truncated, &managedGitError{exitCode: exitCode, err: err}
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
		return stdout.Bytes(), stdout.truncated, errors.Join(ErrManagedGit, ctx.Err())
	}
}

func (r managedGitRunner) captureInput(directory string, input []byte, args ...string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), managedGitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", r.arguments(args...)...)
	cmd.Dir = directory
	cmd.Env = managedGitEnvironment(cmd.Environ())
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr limitedManagedGitBuffer
	stdout.limit = managedGitOutput
	stderr.limit = managedGitOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return stdout.Bytes(), stdout.truncated, errors.Join(ErrManagedGit, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil && !stdout.truncated {
			return stdout.Bytes(), false, nil
		}
		if err == nil {
			return stdout.Bytes(), true, errors.Join(ErrManagedGit, errors.New("managed Git output exceeded limit"))
		}
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return stdout.Bytes(), stdout.truncated, &managedGitError{exitCode: exitCode, err: err}
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
		return stdout.Bytes(), stdout.truncated, errors.Join(ErrManagedGit, ctx.Err())
	}
}

func (r managedGitRunner) arguments(args ...string) []string {
	gitArgs := []string{
		"--no-optional-locks",
		"-c", "core.hooksPath=" + r.hooksPath,
		"-c", "core.fsmonitor=false",
		"-c", "core.untrackedCache=false",
		"-c", "core.preloadIndex=false",
	}
	return append(gitArgs, args...)
}

func managedGitEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment)+8)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"GIT_EDITOR=true",
		"GIT_SEQUENCE_EDITOR=true",
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
	)
}

type limitedManagedGitBuffer struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedManagedGitBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - b.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.Buffer.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

var _ io.Writer = (*limitedManagedGitBuffer)(nil)
