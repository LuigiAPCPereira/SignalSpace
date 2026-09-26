package capability

import "testing"

func TestLegacyOAuthScopesAdaptToInternalCapabilities(t *testing.T) {
	tests := []struct {
		scope string
		want  Capability
	}{
		{OAuthScopeWorkspaceRead, WorkspaceRead},
		{OAuthScopeWorkspaceWrite, WorkspaceWrite},
		{OAuthScopeGitReview, GitReview},
		{OAuthScopeGitIndex, GitIndex},
		{OAuthScopeGitCommit, GitCommit},
		{OAuthScopeTestRun, TestRun},
	}
	for _, test := range tests {
		got, ok := FromOAuthScope(test.scope)
		if !ok || got != test.want {
			t.Errorf("FromOAuthScope(%q) = %q, %v; want %q, true", test.scope, got, ok, test.want)
		}
		scope, ok := OAuthScope(test.want)
		if !ok || scope != test.scope {
			t.Errorf("OAuthScope(%q) = %q, %v; want %q, true", test.want, scope, ok, test.scope)
		}
	}
}

func TestFutureCapabilitiesAreKnownButHaveNoOAuthAdapter(t *testing.T) {
	for _, value := range []Capability{WorkspaceDelete, GitBranch, GitRemoteFetch, GitRemotePush, GitDestructive, ShellExec} {
		if !IsKnown(value) {
			t.Errorf("future capability %q is not in the closed catalog", value)
		}
		if scope, ok := OAuthScope(value); ok || scope != "" {
			t.Errorf("future capability %q unexpectedly has OAuth scope %q", value, scope)
		}
	}
	if _, ok := FromOAuthScope("signalspace:unknown"); ok {
		t.Fatal("unknown OAuth scope was accepted")
	}
}
