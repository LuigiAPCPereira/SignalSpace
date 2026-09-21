package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/SignalSpace/internal/programming"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type harnessTestRunner struct {
	grants      *workspace.Grants
	timeout     time.Duration
	outputLimit int
	starts      atomic.Int32
}

func (r *harnessTestRunner) RunTests(ctx context.Context, owner, clientID, sessionID string) (programming.TestResult, error) {
	var result programming.TestResult
	err := r.grants.WithAuthorizedTestProcessDir(owner, clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		r.starts.Add(1)
		var err error
		result, err = programming.RunPredefinedTest(ctx, directory, r.timeout, r.outputLimit)
		return err
	})
	return result, err
}

func TestRunWorkspaceTestsMCPVerticalFlowAndIndependentRevocation(t *testing.T) {
	key, verifier, _, publicServer := setupOAuth(t)
	root := testRunnerFixture(t, false)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	clientID := strings.Repeat("T", 32)
	sessionID, err := grants.GrantWithScopes(root, clientID, workspace.ScopeRead, workspace.ScopeWrite, workspace.ScopeGit, workspace.ScopeTest)
	if err != nil {
		t.Fatal(err)
	}
	var baseline programming.GitSnapshot
	err = grants.WithAuthorizedGitProcessDir("owner-test", clientID, sessionID, func(directory workspace.ProcessDirectory) error {
		var err error
		baseline, err = programming.CaptureGitSnapshot(context.Background(), directory, 8192)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := &harnessTestRunner{grants: grants, timeout: 30 * time.Second, outputLimit: 8192}
	handler, err := NewOAuthHandler(OAuthConfig{
		ResourceURL:     testResource,
		Issuer:          testIssuer,
		OwnerSubject:    "owner-test",
		workspaceWriter: grants,
		gitReviewer:     &harnessGitReviewer{grants: grants, before: baseline, outputLimit: 8192},
		testRunner:      runner,
	}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	testToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+testRunScope)
	writeToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+workspaceWriteScope)
	gitToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+gitReviewScope)
	readToken := signedWriteToken(t, key, clientID, diagnosticScope+" "+workspaceReadScope)

	metadataResponse, metadata := oauthRequest(t, server, http.MethodGet, metadataPath, "", "", nil)
	if metadataResponse.StatusCode != http.StatusOK || !containsString(metadata["scopes_supported"].([]any), testRunScope) {
		t.Fatalf("isolated metadata omitted test scope: %v", metadata)
	}

	for name, token := range map[string]string{
		"read-only":  readToken,
		"write-only": writeToken,
		"git-only":   gitToken,
	} {
		t.Run("tools list without test scope/"+name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
			if res.StatusCode != http.StatusOK || containsTool(t, result, testRunToolName) {
				t.Fatalf("test tool leaked without scope: %d %v", res.StatusCode, result)
			}
		})
	}
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", testToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != http.StatusOK || !containsTool(t, result, testRunToolName) {
		t.Fatalf("test tool missing with independent scope: %d %v", res.StatusCode, result)
	}

	editResponse, editResult := oauthRequest(t, server, http.MethodPost, "/mcp", writeToken, writeCall(sessionID, "editable.txt", "before\n", "after\n"), nil)
	if editResponse.StatusCode != http.StatusOK || editResult["result"].(map[string]any)["isError"] != false {
		t.Fatalf("MCP edit failed before test execution: %d %v", editResponse.StatusCode, editResult)
	}

	runResponse, runResult := oauthRequest(t, server, http.MethodPost, "/mcp", testToken, testWorkspaceCall(sessionID), nil)
	runText, runError := readResult(t, runResult)
	if runResponse.StatusCode != http.StatusOK || runError {
		t.Fatalf("authorized test execution failed: %d %v", runResponse.StatusCode, runResult)
	}
	var passed testRunResult
	if err := json.Unmarshal([]byte(runText), &passed); err != nil {
		t.Fatal(err)
	}
	if passed.Status != string(programming.TestPassed) || passed.ExitCode != 0 || !passed.Terminated || passed.TimedOut || passed.Canceled || passed.OutputTruncated || len(passed.Command) != 3 || strings.Join(passed.Command, " ") != "go test ./..." {
		t.Fatalf("unexpected passing result: %+v", passed)
	}

	reviewResponse, reviewResult := oauthRequest(t, server, http.MethodPost, "/mcp", gitToken, gitReviewCall(sessionID), nil)
	reviewText, reviewError := readResult(t, reviewResult)
	if reviewResponse.StatusCode != http.StatusOK || reviewError || !strings.Contains(reviewText, "+after") {
		t.Fatalf("vertical edit/test/review did not expose real diff: %d %v", reviewResponse.StatusCode, reviewResult)
	}

	failingTest := "package fixture\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) { t.Fatal(\"expected fixture failure\") }\n"
	if err := grants.ReplaceText("owner-test", clientID, sessionID, "fixture_test.go", fileText(t, filepath.Join(root, "fixture_test.go")), failingTest); err != nil {
		t.Fatal(err)
	}
	runResponse, runResult = oauthRequest(t, server, http.MethodPost, "/mcp", testToken, testWorkspaceCall(sessionID), nil)
	runText, runError = readResult(t, runResult)
	if runResponse.StatusCode != http.StatusOK || runError {
		t.Fatalf("failing test was treated as transport failure: %d %v", runResponse.StatusCode, runResult)
	}
	var failed testRunResult
	if err := json.Unmarshal([]byte(runText), &failed); err != nil {
		t.Fatal(err)
	}
	if failed.Status != string(programming.TestFailed) || failed.ExitCode == 0 || !failed.Terminated || failed.TimedOut || failed.Canceled || !strings.Contains(failed.Stdout+failed.Stderr, "expected fixture failure") {
		t.Fatalf("unexpected failing result: %+v", failed)
	}

	if runner.starts.Load() != 2 {
		t.Fatalf("unexpected process start count before negatives: %d", runner.starts.Load())
	}
	for name, token := range map[string]string{
		"read-only":  readToken,
		"write-only": writeToken,
		"git-only":   gitToken,
	} {
		t.Run("call without test scope/"+name, func(t *testing.T) {
			res, result := oauthRequest(t, server, http.MethodPost, "/mcp", token, testWorkspaceCall(sessionID), nil)
			text, isError := readResult(t, result)
			if res.StatusCode != http.StatusOK || !isError || text != "Test execution authorization required." {
				t.Fatalf("scope negative was not denied: %d %v", res.StatusCode, result)
			}
		})
	}
	clientMismatchToken := signedWriteToken(t, key, strings.Repeat("U", 32), diagnosticScope+" "+testRunScope)
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", clientMismatchToken, testWorkspaceCall(sessionID), nil)
	if text, isError := readResult(t, result); res.StatusCode != http.StatusOK || !isError || text != "Test execution unavailable or not authorized." {
		t.Fatalf("client mismatch reached execution: %d %v", res.StatusCode, result)
	}
	wrongSession := strings.Repeat("s", len(sessionID))
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", testToken, testWorkspaceCall(wrongSession), nil)
	if text, isError := readResult(t, result); res.StatusCode != http.StatusOK || !isError || text != "Test execution unavailable or not authorized." {
		t.Fatalf("session mismatch reached execution: %d %v", res.StatusCode, result)
	}
	extraRequest := testWorkspaceCallWithArguments(map[string]any{"session_id": sessionID, "extra": true})
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", testToken, extraRequest, nil)
	if res.StatusCode != http.StatusOK || result["error"] == nil {
		t.Fatalf("extra argument was accepted: %d %v", res.StatusCode, result)
	}
	if runner.starts.Load() != 2 {
		t.Fatalf("negative calls started a process: %d", runner.starts.Load())
	}

	if err := grants.Revoke(sessionID); err != nil {
		t.Fatal(err)
	}
	res, result = oauthRequest(t, server, http.MethodPost, "/mcp", testToken, testWorkspaceCall(sessionID), nil)
	if text, isError := readResult(t, result); res.StatusCode != http.StatusOK || !isError || text != "Test execution unavailable or not authorized." {
		t.Fatalf("revoked grant remained usable: %d %v", res.StatusCode, result)
	}
	if runner.starts.Load() != 2 {
		t.Fatalf("revoked call started a process: %d", runner.starts.Load())
	}

	res, result = oauthRequest(t, publicServer, http.MethodPost, "/mcp", testToken, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, nil)
	if res.StatusCode != http.StatusOK || containsTool(t, result, testRunToolName) {
		t.Fatalf("public composition exposed test execution: %d %v", res.StatusCode, result)
	}
	res, result = oauthRequest(t, publicServer, http.MethodPost, "/mcp", testToken, testWorkspaceCall(sessionID), nil)
	if res.StatusCode != http.StatusOK || result["error"] == nil {
		t.Fatalf("public composition accepted test execution: %d %v", res.StatusCode, result)
	}
}

func TestRunWorkspaceTestsMCPReportsTimeout(t *testing.T) {
	key, verifier, _, _ := setupOAuth(t)
	root := testRunnerFixture(t, true)
	grants, err := workspace.NewGrants("owner-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = grants.Close() })
	clientID := strings.Repeat("V", 32)
	sessionID, err := grants.GrantWithScopes(root, clientID, workspace.ScopeTest)
	if err != nil {
		t.Fatal(err)
	}
	runner := &harnessTestRunner{grants: grants, timeout: 20 * time.Millisecond, outputLimit: 4096}
	handler, err := NewOAuthHandler(OAuthConfig{ResourceURL: testResource, Issuer: testIssuer, OwnerSubject: "owner-test", testRunner: runner}, verifier)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	token := signedWriteToken(t, key, clientID, diagnosticScope+" "+testRunScope)
	res, result := oauthRequest(t, server, http.MethodPost, "/mcp", token, testWorkspaceCall(sessionID), nil)
	text, isError := readResult(t, result)
	if res.StatusCode != http.StatusOK || isError {
		t.Fatalf("timeout was treated as transport failure: %d %v", res.StatusCode, result)
	}
	var timedOut testRunResult
	if err := json.Unmarshal([]byte(text), &timedOut); err != nil {
		t.Fatal(err)
	}
	if timedOut.Status != string(programming.TestTimedOut) || !timedOut.TimedOut || !timedOut.Terminated || timedOut.Canceled {
		t.Fatalf("timeout state was not observable: %+v", timedOut)
	}
}

func testWorkspaceCall(sessionID string) string {
	return testWorkspaceCallWithArguments(map[string]any{"session_id": sessionID})
}

func testWorkspaceCallWithArguments(arguments map[string]any) string {
	params, _ := json.Marshal(map[string]any{"name": testRunToolName, "arguments": arguments})
	request, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": json.RawMessage(params)})
	return string(request)
}

func containsTool(t *testing.T, result map[string]any, name string) bool {
	t.Helper()
	for _, raw := range result["result"].(map[string]any)["tools"].([]any) {
		if raw.(map[string]any)["name"] == name {
			return true
		}
	}
	return false
}

func containsString(values []any, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func testRunnerFixture(t *testing.T, sleeps bool) string {
	t.Helper()
	root := t.TempDir()
	writeFixtureFile(t, filepath.Join(root, "go.mod"), "module fixture\n\ngo 1.23\n")
	testBody := "package fixture\n\nimport \"testing\"\n\nfunc TestFixture(t *testing.T) {}\n"
	if sleeps {
		testBody = "package fixture\n\nimport (\"testing\"; \"time\")\n\nfunc TestFixture(t *testing.T) { time.Sleep(2 * time.Second) }\n"
	}
	writeFixtureFile(t, filepath.Join(root, "fixture_test.go"), testBody)
	writeFixtureFile(t, filepath.Join(root, "editable.txt"), "before\n")
	runTestFixtureGit(t, root, "init", "-q")
	runTestFixtureGit(t, root, "config", "user.email", "fixture@example.invalid")
	runTestFixtureGit(t, root, "config", "user.name", "SignalSpace fixture")
	runTestFixtureGit(t, root, "add", "go.mod", "fixture_test.go", "editable.txt")
	runTestFixtureGit(t, root, "commit", "-qm", "baseline")
	return root
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func runTestFixtureGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
