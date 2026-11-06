package termprompt

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// aim is a's keystrokes from the cursor's row to its option's row on screen; false
// when either row is missing or ambiguous, so an answer is never typed blind.
func aim(p spec.TerminalPromptSpec, a spec.TerminalPromptAnswerSpec, screen string) (string, bool) {
	if a.Option == "" {
		return a.Keys, true
	}
	cursor, option := -1, -1
	want := squeeze(a.Option)
	for i, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, p.Cursor) {
			if cursor >= 0 {
				return "", false
			}
			cursor = i
		}
		if squeeze(line) != "" && strings.Contains(squeeze(line), want) {
			if option >= 0 {
				return "", false
			}
			option = i
		}
	}
	if cursor < 0 || option < 0 {
		return "", false
	}
	if option >= cursor {
		return strings.Repeat(p.Down, option-cursor) + a.Keys, true
	}
	return strings.Repeat(p.Up, cursor-option) + a.Keys, true
}
