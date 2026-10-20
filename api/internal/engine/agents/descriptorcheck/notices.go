package descriptorcheck

import (
	"fmt"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// noticeRules check that every terminal notice can tell its banner from a
// quote of it: it matches only behind its anchor.
func noticeRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	for i, n := range d.TerminalNotices {
		if n.Anchor != "" {
			continue
		}
		out = append(out, doc.finding("terminal_notices.anchor_missing", SeverityError, fmt.Sprintf("terminal_notices[%d]", i),
			"a notice with no anchor never matches: a turn it ends stays open",
			"declare anchor: the glyph or literal text the banner's own screen row opens with, from a captured screen"))
	}
	return out
}
