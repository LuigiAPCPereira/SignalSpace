package main

// quickModeArgs aceita somente as combinações explícitas e na ordem documentada.
// O modo existente continua terminal-only; nenhum ambiente liga o painel.
func quickModeArgs(args []string) (mode compositionMode, panel, ok bool) {
	if len(args) < 2 || len(args) > 4 || args[0] != "connect" || args[1] != "quick" {
		return compositionInvalid, false, false
	}
	switch len(args) {
	case 2:
		return compositionDiagnostic, false, true
	case 3:
		switch args[2] {
		case "read":
			return compositionRead, false, true
		case "panel":
			return compositionDiagnostic, true, true
		}
	case 4:
		if args[2] == "read" && args[3] == "panel" {
			return compositionRead, true, true
		}
	}
	return compositionInvalid, false, false
}
