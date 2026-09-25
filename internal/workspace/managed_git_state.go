package workspace

import "strings"

const managedWorkspaceRefPrefix = "refs/signalspace/workspaces/"

type managedGitState struct {
	HeadSHA         string
	PrivateRefSHA   string
	PrivateRefSeen  bool
	HasLocalCommits bool
}

func containsManagedScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func managedHeadRef(workspaceID string) string {
	return managedWorkspaceRefPrefix + workspaceID + "/head"
}

func (m *ManagedWorktreeManager) inspectManagedGitState(record *managedWorkspaceMetadata) (managedGitState, error) {
	if record == nil || !validWorkspaceID(record.WorkspaceID) || !validObjectID(record.BaseSHA) {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}
	if output, err := m.runner.capture(record.ManagedRoot, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil || strings.TrimSpace(output) != "" {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	} else if code, ok := managedGitExitCode(err); !ok || code != 1 {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}

	headBytes, headTruncated, err := m.runner.captureOutput(record.ManagedRoot, 128, "rev-parse", "--verify", "--end-of-options", "HEAD^{commit}")
	if err != nil || headTruncated {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}
	head := strings.TrimSpace(string(headBytes))
	if !validObjectID(head) {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}
	base := strings.TrimSpace(record.BaseSHA)
	if output, err := m.runner.capture(record.ManagedRoot, "merge-base", "--is-ancestor", base, head); err != nil || strings.TrimSpace(output) != "" {
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	}

	ref := managedHeadRef(record.WorkspaceID)
	refSeen, err := m.managedRefExists(record.ManagedRoot, ref)
	if err != nil {
		return managedGitState{}, err
	}
	refSHA := ""
	if refSeen {
		refBytes, refTruncated, refErr := m.runner.captureOutput(record.ManagedRoot, 128, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
		if refErr != nil || refTruncated {
			return managedGitState{}, ErrManagedWorkspaceInconsistent
		}
		refSHA = strings.TrimSpace(string(refBytes))
		if !validObjectID(refSHA) {
			return managedGitState{}, ErrManagedWorkspaceInconsistent
		}
	}

	state := managedGitState{HeadSHA: head, PrivateRefSHA: refSHA, PrivateRefSeen: refSeen, HasLocalCommits: head != base}
	switch {
	case !state.HasLocalCommits && refSeen:
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	case state.HasLocalCommits && !refSeen:
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	case state.HasLocalCommits && refSHA != head:
		return managedGitState{}, ErrManagedWorkspaceInconsistent
	case state.HasLocalCommits:
		parents, err := m.runner.capture(record.ManagedRoot, "rev-list", "--parents", "--ancestry-path", base+".."+head)
		if err != nil {
			return managedGitState{}, ErrManagedWorkspaceInconsistent
		}
		for _, line := range strings.Split(strings.TrimSpace(parents), "\n") {
			if line == "" {
				continue
			}
			if len(strings.Fields(line)) > 2 {
				return managedGitState{}, ErrManagedWorkspaceInconsistent
			}
		}
	}
	return state, nil
}

// managedRefExists separa "ref ausente" de "ref existente apontando para um
// objeto que não é commit". Rev-parse com ^{commit} sozinho transforma os
// dois casos no mesmo erro e poderia aceitar uma corrupção como estado inicial.
func (m *ManagedWorktreeManager) managedRefExists(directory, ref string) (bool, error) {
	output, err := m.runner.capture(directory, "show-ref", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}
	if code, ok := managedGitExitCode(err); ok && code == 1 && strings.TrimSpace(output) == "" {
		return false, nil
	}
	return false, ErrManagedWorkspaceInconsistent
}
