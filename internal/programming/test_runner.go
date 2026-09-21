// Package programming contém capacidades locais de programação ainda não
// publicadas como ferramentas MCP.
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

const (
	defaultTestTimeout = 30 * time.Second
	defaultOutputLimit = 256 << 10
	maxTestTimeout     = 5 * time.Minute
	maxOutputLimit     = 1 << 20
)

var ErrInvalidTestExecution = errors.New("invalid local test execution")

type TestStatus string

const (
	TestPassed           TestStatus = "TEST_PASSED"
	TestFailed           TestStatus = "TEST_FAILED"
	TestTimedOut         TestStatus = "TIMED_OUT"
	TestCanceled         TestStatus = "CANCELED"
	TestExecutionUnknown TestStatus = "UNKNOWN"
)

// TestResult registra somente o comando de teste fixo e sua saída limitada.
// O comando não recebe shell, texto livre nem argumentos do MCP.
type TestResult struct {
	Command         []string
	ExitCode        int
	Stdout          string
	Stderr          string
	OutputTruncated bool
	TimedOut        bool
	Canceled        bool
	Terminated      bool
}

// RunPredefinedTest executa apenas `go test ./...` no diretório autorizado.
// A porta deve ter sido criada e concedida localmente; a raiz não funciona
// como sandbox de processo, portanto o processo mantém os privilégios do
// usuário. Falha de teste é devolvida em ExitCode, não como erro de transporte.
func RunPredefinedTest(ctx context.Context, directory workspace.ProcessDirectory, timeout time.Duration, outputLimit int) (TestResult, error) {
	if ctx == nil || directory == nil {
		return TestResult{}, ErrInvalidTestExecution
	}
	if timeout == 0 {
		timeout = defaultTestTimeout
	}
	if timeout < 0 || timeout > maxTestTimeout {
		return TestResult{}, ErrInvalidTestExecution
	}
	if outputLimit == 0 {
		outputLimit = defaultOutputLimit
	}
	if outputLimit < 1 || outputLimit > maxOutputLimit {
		return TestResult{}, ErrInvalidTestExecution
	}
	var result TestResult
	err := directory.WithProcessDir(func(dir string) error {
		var runErr error
		result, runErr = runPredefinedTestInDir(ctx, dir, timeout, outputLimit)
		return runErr
	})
	return result, err
}

func runPredefinedTestInDir(ctx context.Context, dir string, timeout time.Duration, outputLimit int) (TestResult, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout := &limitedBuffer{limit: outputLimit}
	stderr := &limitedBuffer{limit: outputLimit}
	command := []string{"go", "test", "./..."}
	cmd := exec.Command("go", "test", "./...")
	cmd.Dir = dir
	cmd.Env = safeTestEnvironment(cmd.Environ())
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	result := TestResult{Command: append([]string(nil), command...)}
	if err := cmd.Start(); err != nil {
		return result, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		result.Terminated = true
		if waitErr != nil && cmd.ProcessState == nil {
			return result, waitErr
		}
	case <-runCtx.Done():
		result.Canceled = errors.Is(runCtx.Err(), context.Canceled)
		result.TimedOut = errors.Is(runCtx.Err(), context.DeadlineExceeded)
		// O grupo inclui compiladores/testes filhos do `go test`; Wait abaixo
		// confirma que o processo principal encerrou antes do retorno.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Process.Kill()
		<-done
		result.Terminated = true
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = -1
	}
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.OutputTruncated = stdout.truncated || stderr.truncated
	return result, nil
}

type limitedBuffer struct {
	data      []byte
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(data) > remaining {
			b.data = append(b.data, data[:remaining]...)
			b.truncated = true
		} else {
			b.data = append(b.data, data...)
		}
	} else if len(data) > 0 {
		b.truncated = true
	}
	return len(data), nil
}

func (b *limitedBuffer) String() string { return string(b.data) }

func (r TestResult) Status() TestStatus {
	if r.TimedOut {
		return TestTimedOut
	}
	if r.Canceled {
		return TestCanceled
	}
	if !r.Terminated {
		return TestExecutionUnknown
	}
	if r.ExitCode == 0 {
		return TestPassed
	}
	return TestFailed
}

// safeTestEnvironment impede que configurações herdadas troquem o executor,
// o arquivo de módulos ou promovam downloads automáticos durante a fixture.
// Isso não é sandbox: o processo continua com os privilégios do usuário.
func safeTestEnvironment(environment []string) []string {
	blocked := map[string]struct{}{
		"GOFLAGS":     {},
		"GOTOOLCHAIN": {},
		"GOPROXY":     {},
		"GOSUMDB":     {},
		"GONOSUMDB":   {},
		"GOPRIVATE":   {},
		"GONOPROXY":   {},
		"GOMODCACHE":  {},
		"GOTOOLDIR":   {},
		"GOVCS":       {},
		"GOWORK":      {},
	}
	filtered := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := blocked[key]; ok {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off")
}
