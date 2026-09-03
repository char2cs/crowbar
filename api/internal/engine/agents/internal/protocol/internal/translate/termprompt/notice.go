package termprompt

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const maxNoticeRows = 8

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
