package agents

import (
	"slices"
	"strings"

	"github.com/rivo/uniseg"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const (
	pasteStart = "\x1b[200~"
	pasteEnd   = "\x1b[201~"
)

func (a *agent) PromptType() (PromptTyping, bool) {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.Type == nil {
		return PromptTyping{}, false
	}
	return PromptTyping{
		Paste:             ps.Type.Paste,
		Submit:            ps.Type.Submit,
		ComposerRowPrefix: ps.Type.Composer.RowPrefix,
		ComposerGhost:     ps.Type.Composer.Ghost,
		SkipPrefixes:      slices.Clone(ps.Type.SkipPrefixes),
	}, true
}

// CanType refuses a message holding the end-paste sequence rather than stripping
// it: a message must reach the CLI exactly as written or not at all.
func (a *agent) CanType(text string) bool {
	typing, ok := a.PromptType()
	if !ok || typing.Paste != spec.PasteBracketed || strings.Contains(text, pasteEnd) {
		return false
	}
	return !slices.ContainsFunc(typing.SkipPrefixes, func(prefix string) bool {
		return strings.HasPrefix(text, prefix)
	})
}

// ComposerScreen is the terminal read ComposerEmpty judges: text rows split on
// newlines, a 0-based cursor cell, and Faint as [row][col] over the grid.
type ComposerScreen struct {
	Text                 string
	CursorRow, CursorCol int
	Faint                [][]bool
}

func (a *agent) ComposerEmpty(screen ComposerScreen) bool {
	typing, ok := a.PromptType()
	if !ok {
		return false
	}
	rows := strings.Split(screen.Text, "\n")
	if screen.CursorRow < 0 || screen.CursorRow >= len(rows) {
		return false
	}
	row := rows[screen.CursorRow]
	if !strings.HasPrefix(row, typing.ComposerRowPrefix) || screen.CursorCol != uniseg.StringWidth(typing.ComposerRowPrefix) {
		return false
	}
	var faint []bool
	if screen.CursorRow < len(screen.Faint) {
		faint = screen.Faint[screen.CursorRow]
	}
	return restIsGhost(row[len(typing.ComposerRowPrefix):], screen.CursorCol, faint, typing.ComposerGhost == spec.GhostFaint)
}

// restIsGhost reports whether every cell of rest, from col on, is blank or, when
// ghostFaint, drawn faint: anything else is a draft the cursor sits before.
func restIsGhost(rest string, col int, faint []bool, ghostFaint bool) bool {
	for g := uniseg.NewGraphemes(rest); g.Next(); col += g.Width() {
		if strings.TrimSpace(g.Str()) == "" {
			continue
		}
		if !ghostFaint || col >= len(faint) || !faint[col] {
			return false
		}
	}
	return true
}

func (a *agent) PromptTypeBytes(text string) ([]byte, bool) {
	if !a.CanType(text) {
		return nil, false
	}
	typing, _ := a.PromptType()
	return []byte(pasteStart + text + pasteEnd + typing.Submit), true
}
