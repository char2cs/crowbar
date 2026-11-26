package agents

import (
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func (a *agent) PromptType() (PromptTyping, bool) {
	ps := a.spec.Presentation.PromptSubmit
	if ps == nil || ps.Type == nil {
		return PromptTyping{}, false
	}
	return PromptTyping{
		Input:             ps.Type.Input,
		Submit:            ps.Type.Submit,
		ChunkBytes:        ps.Type.ChunkBytes,
		ChunkInterval:     time.Duration(ps.Type.ChunkIntervalMS) * time.Millisecond,
		ComposerRowPrefix: ps.Type.Composer.RowPrefix,
		ComposerGhost:     ps.Type.Composer.Ghost,
		SkipPrefixes:      slices.Clone(ps.Type.SkipPrefixes),
		SkipSuffixes:      slices.Clone(ps.Type.SkipSuffixes),
	}, true
}

// CanType refuses a message holding a control character other than a newline
// rather than stripping it: it must reach the CLI exactly as written or not at all.
func (a *agent) CanType(text string) bool {
	typing, ok := a.PromptType()
	if !ok || typing.Input != spec.InputKeys || strings.ContainsFunc(text, isKeyControl) {
		return false
	}
	if slices.ContainsFunc(typing.SkipSuffixes, func(suffix string) bool { return strings.HasSuffix(text, suffix) }) {
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

// isKeyControl covers C0, DEL and C1: a terminal may read U+0080-U+009F as a control.
func isKeyControl(r rune) bool { return r != '\n' && (r < 0x20 || (r >= 0x7f && r <= 0x9f)) }

func (a *agent) PromptTypeWrites(text string) ([][]byte, bool) {
	if !a.CanType(text) {
		return nil, false
	}
	typing, _ := a.PromptType()
	var writes [][]byte
	for rest := text; rest != ""; {
		n := min(typing.ChunkBytes, len(rest))
		// Cut on a rune boundary: a split rune would reach the CLI as two garbled keys.
		for n > 1 && n < len(rest) && !utf8.RuneStart(rest[n]) {
			n--
		}
		writes = append(writes, []byte(rest[:n]))
		rest = rest[n:]
	}
	return append(writes, []byte(typing.Submit)), true
}
