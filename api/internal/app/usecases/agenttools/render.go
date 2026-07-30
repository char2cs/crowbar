package agenttools

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// renderThreads emits one anchor row per thread with every message on its own
// indented line.
//
// Keys appear once (in the header), not per row, which is what makes this
// cheaper than JSON or YAML for the same data. Prose is never inlined into a
// row: review bodies are user-authored markdown full of colons, dashes and code
// fences, and inlining them would let a comment corrupt the structure.
func renderThreads(threads []domain.ReviewThread) string {
	if len(threads) == 0 {
		return "No review threads."
	}
	var b strings.Builder
	b.WriteString("id  file:lines  side  state  messages\n")
	for _, t := range threads {
		state := "unresolved"
		if t.IsResolved() {
			state = "resolved"
		}
		fmt.Fprintf(&b, "%s  %s:%d-%d  %s  %s  %d\n",
			t.ID, t.FilePath, t.StartLine, t.EndLine, t.Side, state, len(t.Messages))
		for _, m := range t.Messages {
			author := m.Author
			if m.IsAgent {
				author += " (agent)"
			}
			fmt.Fprintf(&b, "    %s: %s\n", author, m.Body)
		}
	}
	return b.String()
}

func renderScope(base string, files []gitdomain.ReviewFileSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "This review covers everything on this branch since %s.\n", base)
	if len(files) == 0 {
		b.WriteString("No changed files.\n")
		return b.String()
	}
	b.WriteString("status  +adds  -dels  path\n")
	for _, f := range files {
		fmt.Fprintf(&b, "%s  +%d  -%d  %s\n", f.Status, f.Additions, f.Deletions, f.Path)
	}
	return b.String()
}
