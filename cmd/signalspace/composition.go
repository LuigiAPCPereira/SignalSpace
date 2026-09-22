package main

import (
	"errors"

	"github.com/LuigiAPCPereira/SignalSpace/internal/workspace"
)

type compositionMode uint8

const (
	compositionInvalid compositionMode = iota
	compositionDiagnostic
	compositionRead
)

type workspaceConsoleMode uint8

const (
	workspaceConsoleApprovalsOnly workspaceConsoleMode = iota + 1
	workspaceConsoleRead
)

type compositionPlan struct {
	mode               compositionMode
	oauthScope         string
	workspaceReadScope string
	consoleMode        workspaceConsoleMode
}

const compositionDiagnosticScope = "signalspace:diagnostic"

func planComposition(mode compositionMode) (compositionPlan, error) {
	switch mode {
	case compositionDiagnostic:
		return compositionPlan{
			mode:        compositionDiagnostic,
			oauthScope:  compositionDiagnosticScope,
			consoleMode: workspaceConsoleApprovalsOnly,
		}, nil
	case compositionRead:
		return compositionPlan{
			mode:               compositionRead,
			oauthScope:         compositionDiagnosticScope,
			workspaceReadScope: workspace.ScopeRead,
			consoleMode:        workspaceConsoleRead,
		}, nil
	default:
		return compositionPlan{}, errors.New("unsupported SignalSpace composition; allowed modes are diagnostic and read")
	}
}
