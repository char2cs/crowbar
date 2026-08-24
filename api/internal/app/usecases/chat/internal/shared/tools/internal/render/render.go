// Package render turns the records the tool surface serves into the plain text an
// agent reads.
//
// It is pure — values in, a string out — and knows nothing about tools, stores or
// callers. That is what lets the caps below live here rather than in the tool
// surface's limits table: how many messages a thread shows and how far a body is
// truncated are rendering decisions, and they are only meaningful next to the code
// that makes them.
//
// Nothing here truncates silently. Every cap that bites says so in the output, so a
// model reading a shortened list knows it is shortened and can ask for the rest.
package render

const (
	// AllThreadMessages is the message cap for the single-thread view — none. It is
	// spelled rather than passed as a bare 0 because 0 read literally is "render no
	// messages", the exact opposite of what that view is for.
	AllThreadMessages = 0
	// MaxThreadMessages is how many messages of ONE thread are rendered: the
	// root plus the (MaxThreadMessages-1) most recent replies.
	//
	// Those are the two ends that carry the information. The root is the finding
	// itself — what the user actually asked for — and the newest replies are the
	// thread's current state, which is what an agent deciding whether the finding
	// is still open needs. The middle of a long back-and-forth is the part a
	// reader can most afford to lose, so it is the part that goes.
	MaxThreadMessages = 4
	// MaxMessageBodyChars and MaxTurnBodyChars cap ONE free-text body — a review
	// message and a chat turn respectively.
	//
	// The count caps above bound how many ROWS a page has; without these, they
	// bound nothing that matters, because every row carries an arbitrarily long
	// agent-written markdown body. A full list_review_threads page is 20 threads ×
	// up to MaxThreadMessages bodies = 80 bodies, and at the several hundred
	// characters a real finding runs to that page was 12-20k tokens — three to five
	// times the budget the row counts were derived from.
	//
	// So each number is the SAME 16 KB page budget divided by that tool's own
	// worst-case body count: 16384/80 ≈ 200 for a review message, 16384/20 ≈ 800
	// for a chat turn. The two differ by 4× because the surfaces do: a chat turn is
	// model prose and there are twenty of them, a review message is one point in an
	// argument and there can be eighty.
	//
	// 200 characters is about one full sentence, which is deliberately a HEADLINE
	// rather than a whole finding. What makes that acceptable is that a review
	// message is never the only copy of what it says: the anchor row names the file
	// and lines, so an agent that needs more reads the code. Sizing the cap to hold
	// a whole finding instead would mean sizing the page to 80 whole findings, which
	// is the unbounded result this file exists to remove.
	//
	// Characters are counted in RUNES, not bytes: cutting a body mid-UTF-8-sequence
	// would emit a replacement character into tool output that a model reads as
	// content.
	MaxMessageBodyChars = 200
	MaxTurnBodyChars    = 800
	// MaxScopeRangesPerFile and MaxScopeRanges bound the changed line ranges
	// get_review_scope reports beside its files.
	//
	// The ranges are what make the file list anchorable: post_review_comment
	// requires an anchor to sit inside ONE changed range, so a scope naming only
	// paths leaves a model to guess line numbers and be refused. They are also the
	// one part of that reply whose size is driven by the DIFF rather than by the
	// file count — a file rewritten line by line contributes up to
	// MaxOutlineHunksPerFile (1000) ranges on each side — so both a per-file and a
	// per-page bound are needed. One without the other leaves a hole: a per-file
	// cap alone still lets 300 files × 2 sides blow the page, and a page cap alone
	// lets the first file spend all of it.
	//
	// 6 per file per side is the working set of a file a human reads top-down; past
	// that the file is one to open rather than to anchor from a listing, and the
	// row says how many it did not print. 300 for the page is drawn against the same
	// 16 KB budget as every cap above: a range renders as about twelve characters
	// ("1234-1245, "), so 300 of them is ~3.6 KB — roughly what 100 file rows
	// already cost, and a quarter of the page.
	//
	// A page that runs out says which files it stopped at and what offset fetches
	// their ranges, exactly as the row cap says what it dropped: geometry that is
	// silently absent reads as "this file has no changed lines", which is the one
	// conclusion that would stop a model commenting on it at all.
	MaxScopeRangesPerFile = 6
	MaxScopeRanges        = 300
)

// ChatTurn is one recorded conversation turn: the speaker it is attributed to
// ("user", "assistant (claude)") and what was said.
//
// Speaker is already rendered rather than a role/provider pair because the
// attribution wording is the ledger's — it is what has been written into every
// chat log Crowbar has ever produced — and re-deriving it here would be a second
// place for that wording to drift from the first.
type ChatTurn struct {
	Speaker string
	Body    string
}
