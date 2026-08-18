package termprompt

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// maxNoticeRows bounds how much of the screen one notice may carry into a chat.
//
// The measured codex banner is a single sentence of about 190 characters, which
// the TUI wrapped onto three rows at 100 columns. Eight rows covers that same
// sentence down to a 24-column pane — narrower than anything Crowbar's terminal
// is usable at — while bounding the pathological case, which is a CLI that paints
// a full screen of unrelated non-blank rows directly under its banner. Eight rows
// of a provider's own words is a message; eighty is a screen dump.
const maxNoticeRows = 8

// MatchNotice reports whether this screen is showing a notice the provider
// declared, and captures the provider's own sentence when it is.
//
// FIRST DECLARED MATCH WINS, with none of the specific-beats-generic ordering
// Match applies to prompts. That distinction exists because a prompt may be
// declared without a kind — "something is up" is a useful answer about a blocked
// CLI. A notice may not: every one carries a kind, so there is no generic case to
// rank against, and declaration order is the descriptor author's own priority.
//
// A provider that declares nothing never matches, and its chats behave exactly as
// they did before this existed — the same degradation story the prompt needles
// have, and the reason claude declares no notices at all.
func MatchNotice(d *spec.Descriptor, screen string) (models.TerminalNotice, bool) {
	if d == nil || len(d.TerminalNotices) == 0 || screen == "" {
		return models.TerminalNotice{}, false
	}
	rows := strings.Split(screen, "\n")
	haystack, rowOf := squeezeRows(rows)
	if haystack == "" {
		return models.TerminalNotice{}, false
	}

	for _, n := range d.TerminalNotices {
		needle := squeeze(n.Needle)
		if needle == "" {
			continue
		}
		at := strings.Index(haystack, needle)
		if at < 0 {
			continue
		}
		return models.TerminalNotice{
			Kind:     n.Kind,
			Needle:   n.Needle,
			Text:     capture(rows, rowOf[at]),
			EndsTurn: n.EndsTurn,
		}, true
	}
	return models.TerminalNotice{}, false
}

// squeezeRows applies the same reduction squeeze does, but remembers which SCREEN
// ROW every surviving byte came from.
//
// The index is what makes a notice quotable. Matching has to happen across the
// whole squeezed screen — a needle genuinely arrives split across two rows when a
// TUI wraps it, and a per-row search would miss exactly the case this exists for —
// but the ANSWER has to name a row, because what reaches the user is the
// provider's sentence and not the needle that found it. So the match is done on
// the flattened text and then mapped back.
//
// One entry per BYTE rather than per rune: strings.Index answers in bytes, and a
// lowercased non-ASCII letter is several of them.
func squeezeRows(rows []string) (string, []int) {
	var b strings.Builder
	rowOf := make([]int, 0, len(rows)*8)
	for i, row := range rows {
		for _, r := range row {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				continue
			}
			before := b.Len()
			b.WriteRune(unicode.ToLower(r))
			for n := b.Len() - before; n > 0; n-- {
				rowOf = append(rowOf, i)
			}
		}
	}
	return b.String(), rowOf
}

// capture quotes the matched row plus the rows the provider's sentence wrapped
// onto: the following NON-BLANK rows, up to maxNoticeRows in total.
//
// A blank row is the terminator because a blank row is what a TUI puts between
// one block of output and the next. It is not a perfect boundary and it does not
// need to be — over-capturing costs a few extra words of the CLI's own screen in
// the chat, while under-capturing would cut a sentence in half, and the measured
// banner's most useful half (the reset time) is at the END of it.
//
// Rows are trimmed and rejoined with single spaces. The row breaks are the
// TERMINAL'S, chosen by whatever width the pane happened to be, and the leading
// indentation is the TUI's padding — neither is the provider's writing, so
// preserving them would be preserving an artifact rather than the words. What
// stays byte-for-byte is every character the provider actually wrote, including
// its own bullet glyph, its URLs and its reset timestamp.
func capture(rows []string, start int) string {
	parts := make([]string, 0, maxNoticeRows)
	for i := start; i < len(rows) && len(parts) < maxNoticeRows; i++ {
		row := strings.TrimSpace(rows[i])
		if row == "" {
			break
		}
		parts = append(parts, row)
	}
	return strings.Join(parts, " ")
}
