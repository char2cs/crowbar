package termprompt

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const maxNoticeRows = 8

// MatchNotice is the first notice d declares whose anchor opens a raw row of
// screen at its first column with its needle after it, and the provider's own
// sentence from that row. Past the anchor, text compares squeezed and may wrap.
func MatchNotice(d *spec.Descriptor, screen string) (models.TerminalNotice, bool) {
	if d == nil || screen == "" {
		return models.TerminalNotice{}, false
	}
	rows := strings.Split(screen, "\n")
	for _, n := range d.TerminalNotices {
		if text, ok := anchored(rows, n.Anchor, squeeze(n.Needle)); ok {
			return models.TerminalNotice{Kind: n.Kind, Needle: n.Needle, Text: text, EndsTurn: n.EndsTurn}, true
		}
	}
	return models.TerminalNotice{}, false
}

// anchored is the text of the first row anchor opens whose needle follows it;
// an indented row is a quote, as codex indents prompt and reply continuations.
func anchored(rows []string, anchor, needle string) (string, bool) {
	if anchor == "" || needle == "" {
		return "", false
	}
	for i, row := range rows {
		if !strings.HasPrefix(row, anchor) {
			continue
		}
		text := capture(rows, i)
		if strings.HasPrefix(squeeze(strings.TrimPrefix(text, anchor)), needle) {
			return text, true
		}
	}
	return "", false
}

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
