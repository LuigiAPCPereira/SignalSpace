package main

// quickModeArgs aceita somente as combinações explícitas e na ordem documentada.
// O modo existente continua terminal-only; nenhum ambiente liga o painel.
func quickModeArgs(args []string) (read, panel, ok bool) {
	if len(args) < 2 || len(args) > 4 || args[0] != "connect" || args[1] != "quick" {
		return false, false, false
	}
	switch len(args) {
	case 2:
		return false, false, true
	case 3:
		switch args[2] {
		case "read":
			return true, false, true
		case "panel":
			return false, true, true
		}
	case 4:
		if args[2] == "read" && args[3] == "panel" {
			return true, true, true
		}
	}
	return false, false, false
}
