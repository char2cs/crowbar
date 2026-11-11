// Package render turns tool-surface records into the plain text an agent reads. It is pure, so the
// rendering caps live beside the code applying them; every cap that bites says so in the output,
// so a model knows a list is shortened and can ask for the rest.
package render

const (
	// AllThreadMessages is the single-thread view's message cap: none. Named because a bare 0 reads as
	// "render no messages".
	AllThreadMessages = 0
	// MaxThreadMessages is how many messages of one thread are rendered: the root (the finding) plus the
	// most recent replies (its current state). The middle of a long exchange is what a reader can lose.
	MaxThreadMessages = 4
	// MaxMessageBodyChars and MaxTurnBodyChars cap one review message and one chat turn body, in runes:
	// the 16 KB page budget divided by each page's worst-case body count (80 messages, 20 turns). A
	// message is a headline; its anchor row says where to read the rest.
	MaxMessageBodyChars = 200
	MaxTurnBodyChars    = 800
	// MaxScopeRangesPerFile and MaxScopeRanges bound the changed ranges get_review_scope lists (per file
	// per side, and per page; ~3.6 KB of the 16 KB budget). Range count follows the diff, not the file
	// count, so both bounds are needed; a page that runs out says where to resume.
	MaxScopeRangesPerFile = 6
	MaxScopeRanges        = 300
)

// ChatTurn is one recorded turn: its speaker ("user", "assistant (<provider id>)") and what was said.
// Speaker arrives pre-rendered because the attribution wording is the ledger's to own.
type ChatTurn struct {
	Speaker string
	Body    string
}
