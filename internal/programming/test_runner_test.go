package programming

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

func fixtureSession(t *testing.T, testBody string) *workspace.Session {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture_test.go"), []byte(testBody), 0600); err != nil {
		t.Fatal(err)
	}
	session, err := workspace.OpenApprovedRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestRunPredefinedTestReturnsBoundedSuccessfulResult(t *testing.T) {
	session := fixtureSession(t, "package fixture\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) {}\n")
	result, err := RunPredefinedTest(context.Background(), session, 30*time.Second, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Status() != TestPassed || !result.Terminated || result.TimedOut || result.Canceled {
		t.Fatalf("unexpected successful result: %+v", result)
	}
	if len(result.Command) != 3 || result.Command[0] != "go" || result.Command[1] != "test" || result.Command[2] != "./..." {
		t.Fatalf("unexpected command: %#v", result.Command)
	}
	if len(result.Stdout) > 4096 || len(result.Stderr) > 4096 {
		t.Fatal("test output exceeded configured limit")
	}
}

func TestRunPredefinedTestReportsFailureWithoutTreatingItAsTransportError(t *testing.T) {
	session := fixtureSession(t, "package fixture\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) { t.Fatal(\"expected failure\") }\n")
	result, err := RunPredefinedTest(context.Background(), session, 30*time.Second, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 || result.Status() != TestFailed || !result.Terminated || result.TimedOut || result.Canceled {
		t.Fatalf("unexpected failed result: %+v", result)
	}
	if result.Stdout == "" && result.Stderr == "" {
		t.Fatal("failed test returned no bounded diagnostic output")
	}
}

func TestRunPredefinedTestReportsOutputTruncationAndCancellation(t *testing.T) {
	session := fixtureSession(t, "package fixture\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) { t.Fatal(\"long diagnostic output\" + \"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\") }\n")
	truncated, err := RunPredefinedTest(context.Background(), session, 30*time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	if truncated.Status() != TestFailed || !truncated.OutputTruncated {
		t.Fatalf("failure output was not bounded: %+v", truncated)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled, err := RunPredefinedTest(ctx, session, time.Second, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Status() != TestCanceled || !canceled.Canceled || !canceled.Terminated {
		t.Fatalf("cancellation was not observed: %+v", canceled)
	}
}

func TestRunPredefinedTestStopsAtTimeoutAndRejectsInvalidBounds(t *testing.T) {
	session := fixtureSession(t, "package fixture\n\nimport (\"testing\"; \"time\")\n\nfunc TestFixture(t *testing.T) { time.Sleep(2 * time.Second) }\n")
	result, err := RunPredefinedTest(context.Background(), session, 20*time.Millisecond, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != TestTimedOut || !result.TimedOut || !result.Terminated || result.ExitCode == 0 {
		t.Fatalf("timeout was not reported: %+v", result)
	}
	if _, err := RunPredefinedTest(context.Background(), session, -time.Second, 4096); !errors.Is(err, ErrInvalidTestExecution) {
		t.Fatalf("invalid timeout accepted: %v", err)
	}
	if _, err := RunPredefinedTest(context.Background(), session, time.Second, maxOutputLimit+1); !errors.Is(err, ErrInvalidTestExecution) {
		t.Fatalf("invalid output limit accepted: %v", err)
	}
}

func TestRunPredefinedTestRequiresLiveSession(t *testing.T) {
	session := fixtureSession(t, "package fixture\n")
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := RunPredefinedTest(context.Background(), session, time.Second, 4096); !errors.Is(err, workspace.ErrClosed) {
		t.Fatalf("closed session accepted: %v", err)
	}
}
