package main

import (
	"errors"

	"github.com/LuigiAPCPereira/SignalSpace/internal/admin"
	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type compositionMode uint8

const (
	compositionInvalid compositionMode = iota
	compositionDiagnostic
	compositionRead
	compositionProgramming
)

type workspaceConsoleMode uint8

const (
	workspaceConsoleApprovalsOnly workspaceConsoleMode = iota + 1
	workspaceConsoleRead
	workspaceConsoleProgramming
)

type compositionValidatorMode uint8

const (
	compositionLocalOAuthJWTValidator compositionValidatorMode = iota + 1
)

type compositionPlan struct {
	mode                compositionMode
	oauthScope          string
	workspaceReadScope  string
	workspaceWriteScope string
	gitReviewScope      string
	gitIndexScope       string
	gitCommitScope      string
	consoleMode         workspaceConsoleMode
	validatorMode       compositionValidatorMode
	mcpAddress          string
}

const compositionDiagnosticScope = "signalspace:diagnostic"

func planComposition(mode compositionMode) (compositionPlan, error) {
	switch mode {
	case compositionDiagnostic:
		return compositionPlan{
			mode:          compositionDiagnostic,
			oauthScope:    compositionDiagnosticScope,
			consoleMode:   workspaceConsoleApprovalsOnly,
			validatorMode: compositionLocalOAuthJWTValidator,
			mcpAddress:    admin.PublicAddress,
		}, nil
	case compositionRead:
		return compositionPlan{
			mode:               compositionRead,
			oauthScope:         compositionDiagnosticScope,
			workspaceReadScope: workspace.ScopeRead,
			consoleMode:        workspaceConsoleRead,
			validatorMode:      compositionLocalOAuthJWTValidator,
			mcpAddress:         admin.PublicAddress,
		}, nil
	case compositionProgramming:
		return compositionPlan{
			mode:                compositionProgramming,
			oauthScope:          compositionDiagnosticScope,
			workspaceReadScope:  workspace.ScopeRead,
			workspaceWriteScope: workspace.ScopeWrite,
			gitReviewScope:      workspace.ScopeGit,
			gitIndexScope:       workspace.ScopeGitIndex,
			gitCommitScope:      workspace.ScopeGitCommit,
			consoleMode:         workspaceConsoleProgramming,
			validatorMode:       compositionLocalOAuthJWTValidator,
			mcpAddress:          admin.PublicAddress,
		}, nil
	default:
		return compositionPlan{}, errors.New("unsupported SignalSpace composition; allowed modes are diagnostic, read and programming")
	}
}

func validateCompositionPlan(plan compositionPlan) (compositionPlan, error) {
	canonical, err := planComposition(plan.mode)
	if err != nil {
		return compositionPlan{}, err
	}
	if plan != canonical {
		return compositionPlan{}, errors.New("composition plan does not match the closed SignalSpace policy")
	}
	return canonical, nil
}
