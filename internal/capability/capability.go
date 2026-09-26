// Package capability define as autorizações internas do SignalSpace.
//
// Escopos OAuth são adaptados nesta fronteira e não são a autoridade interna
// de uma operação. A lista inclui capabilities futuras para que uma política
// desconhecida possa falhar fechado sem aceitar strings arbitrárias.
package capability

// Capability é uma autorização interna, independente da composição OAuth.
type Capability string

const (
	WorkspaceRead   Capability = "workspace.read"
	WorkspaceWrite  Capability = "workspace.write"
	WorkspaceDelete Capability = "workspace.delete"

	GitReview      Capability = "git.review"
	GitIndex       Capability = "git.index"
	GitCommit      Capability = "git.commit"
	GitBranch      Capability = "git.branch"
	GitRemoteFetch Capability = "git.remote.fetch"
	GitRemotePush  Capability = "git.remote.push"
	GitDestructive Capability = "git.destructive"

	TestRun   Capability = "test.run"
	ShellExec Capability = "shell.exec"
)

const (
	OAuthScopeWorkspaceRead  = "signalspace:workspace.read"
	OAuthScopeWorkspaceWrite = "signalspace:workspace.write"
	OAuthScopeGitReview      = "signalspace:git.review"
	OAuthScopeGitIndex       = "signalspace:git.index"
	OAuthScopeGitCommit      = "signalspace:git.commit"
	OAuthScopeTestRun        = "signalspace:test.run"
)

var known = map[Capability]struct{}{
	WorkspaceRead: {}, WorkspaceWrite: {}, WorkspaceDelete: {},
	GitReview: {}, GitIndex: {}, GitCommit: {}, GitBranch: {},
	GitRemoteFetch: {}, GitRemotePush: {}, GitDestructive: {},
	TestRun: {}, ShellExec: {},
}

// IsKnown reports whether capability belongs to the closed internal catalog.
func IsKnown(value Capability) bool {
	_, ok := known[value]
	return ok
}

// FromOAuthScope adapts the six legacy/public scopes into internal
// capabilities. Unknown scopes are deliberately not guessed or accepted.
func FromOAuthScope(scope string) (Capability, bool) {
	switch scope {
	case OAuthScopeWorkspaceRead:
		return WorkspaceRead, true
	case OAuthScopeWorkspaceWrite:
		return WorkspaceWrite, true
	case OAuthScopeGitReview:
		return GitReview, true
	case OAuthScopeGitIndex:
		return GitIndex, true
	case OAuthScopeGitCommit:
		return GitCommit, true
	case OAuthScopeTestRun:
		return TestRun, true
	default:
		return "", false
	}
}

// OAuthScope adapts only capabilities with an existing public scope. Future
// capabilities remain internal until a separate contract promotes them.
func OAuthScope(value Capability) (string, bool) {
	switch value {
	case WorkspaceRead:
		return OAuthScopeWorkspaceRead, true
	case WorkspaceWrite:
		return OAuthScopeWorkspaceWrite, true
	case GitReview:
		return OAuthScopeGitReview, true
	case GitIndex:
		return OAuthScopeGitIndex, true
	case GitCommit:
		return OAuthScopeGitCommit, true
	case TestRun:
		return OAuthScopeTestRun, true
	default:
		return "", false
	}
}

// LegacyCapabilities returns the canonical order used by the existing grant
// adapter and its public snapshots.
func LegacyCapabilities() []Capability {
	return []Capability{WorkspaceRead, WorkspaceWrite, GitReview, GitIndex, GitCommit, TestRun}
}

// Catalog returns every capability known to the local domain in stable order.
// Future entries are catalogued for fail-closed policy reasoning only; they do
// not imply a tool, executor, OAuth scope or public grant.
func Catalog() []Capability {
	return []Capability{
		WorkspaceRead, WorkspaceWrite, WorkspaceDelete,
		GitReview, GitIndex, GitCommit, GitBranch,
		GitRemoteFetch, GitRemotePush, GitDestructive,
		TestRun, ShellExec,
	}
}
