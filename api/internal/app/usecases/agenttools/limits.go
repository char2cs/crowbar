package agenttools

import "fmt"

// The tool surface is BOUNDED: every list a tool renders is capped, and every
// cap states in the rendered text what it left out and how to reach it.
//
// The budget these numbers are drawn against is ONE codex turn. codex does not
// defer MCP tool schemas, so all eight tools' descriptions and schemas are
// resident on every turn before a single result is read — which is why the tool
// ceiling exists at all, and why a result has to be small enough to sit beside
// seven other schemas AND the model's actual work. The target is roughly 4k
// tokens (~16 KB) for a FULL page of any one tool: about 2% of a 200k window,
// small enough that an agent can call several tools in one turn without the
// results crowding out its reasoning.
//
// Silent truncation is the specific failure being designed against. A model
// handed a short list reads it as the whole list and then acts confidently on a
// partial picture — worse than an error, which it would at least try to recover
// from. So nothing here truncates quietly: each tool leads its output with a
// note giving the range, the total, and the argument that fetches the rest.
const (
	// defaultThreadPage / maxThreadPage bound one page of list_review_threads.
	//
	// 20 threads is the working set of a review a human would actually sit
	// through, and at an anchor row plus up to maxThreadMessages short messages
	// each it lands near the 4k-token target. maxThreadPage exists because limit
	// is a MODEL-supplied number: without a ceiling, limit=100000 is one
	// argument away from the unbounded result this whole file removes.
	defaultThreadPage = 20
	maxThreadPage     = 50

	// maxThreadMessages is how many messages of ONE thread are rendered: the
	// root plus the (maxThreadMessages-1) most recent replies.
	//
	// Those are the two ends that carry the information. The root is the finding
	// itself — what the user actually asked for — and the newest replies are the
	// thread's current state, which is what an agent deciding whether the finding
	// is still open needs. The middle of a long back-and-forth is the part a
	// reader can most afford to lose, so it is the part that goes.
	maxThreadMessages = 4

	// defaultChatLogTurns / maxChatLogTurns bound get_chat_log.
	//
	// A turn is a whole message — an assistant turn can be hundreds of tokens on
	// its own — so this cap is set well below the thread cap: 20 turns is roughly
	// ten exchanges, enough to see what a sibling chat is currently doing, at
	// perhaps 3k tokens. Reading a sibling's ENTIRE history is not what this tool
	// is for, which is why the ceiling stays modest even when a model asks.
	defaultChatLogTurns = 20
	maxChatLogTurns     = 50

	// defaultScopeFiles / maxScopeFiles bound get_review_scope's changed-file
	// list.
	//
	// A file row is one short line (status, two counts, a path), so files are an
	// order of magnitude cheaper than messages and the cap can be an order of
	// magnitude higher: 100 rows is around 1.5k tokens and covers essentially
	// every branch a human reviews. The cap still has to exist — a
	// dependency-lockfile refresh or a generated-code drop is thousands of files,
	// and that is exactly the review where an agent needs the tool to stay usable.
	defaultScopeFiles = 100
	maxScopeFiles     = 300

	// maxMessageBodyChars and maxTurnBodyChars cap ONE free-text body — a review
	// message and a chat turn respectively.
	//
	// The count caps above bound how many ROWS a page has; without these, they
	// bound nothing that matters, because every row carries an arbitrarily long
	// agent-written markdown body. A full list_review_threads page is 20 threads ×
	// up to maxThreadMessages bodies = 80 bodies, and at the several hundred
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
	maxMessageBodyChars = 200
	maxTurnBodyChars    = 800

	// maxAnchorRangesShown bounds how many changed ranges a rejected anchor is
	// told about.
	//
	// A rejection has to name the legal move or the model guesses the same wrong
	// number again, and the legal moves here are the file's changed ranges. But a
	// single file may contribute up to a thousand hunks to an outline, and an error
	// carrying a thousand ranges is a page of tool output spent on a failed call.
	//
	// 8 is about seventy characters — small beside the sentence explaining the rule
	// — and it is enough because the ranges chosen are the ones NEAREST the anchor
	// the model actually tried (see nearestRanges), not the first eight in the file.
	// A model anchoring on a function it just read is within a hunk or two of the
	// right answer; it does not need the other side of the file.
	maxAnchorRangesShown = 8

	// allThreadMessages is the message cap for the single-thread view — none. It is
	// spelled rather than passed as a bare 0 because 0 read literally is "render no
	// messages", the exact opposite of what that view is for.
	allThreadMessages = 0
)

// window is the slice of a list one tool call renders, plus the total it was cut
// from. The total is carried alongside because it is what the truncation note
// reports: a window with no memory of what it came from cannot say what it left
// out, and a cap that cannot say what it left out is a silent one.
type window struct {
	start int
	end   int
	total int
}

func (w window) complete() bool {
	return w.start == 0 && w.end == w.total
}

func (w window) empty() bool {
	return w.start >= w.end
}

// forwardWindow is the ordinary offset/limit page, counted from the start of the
// list. Both arguments come from a model, so both are clamped rather than
// trusted: a negative offset reads as "from the beginning" and one past the end
// yields an empty page the caller reports as such, never an out-of-range slice.
func forwardWindow(
	total int,
	offset int,
	limit int,
	fallback int,
	ceiling int,
) window {
	size := clampLimit(limit, fallback, ceiling)
	start := clamp(offset, 0, total)
	end := start + size
	if end > total {
		end = total
	}
	return window{start: start, end: end, total: total}
}

// recentWindow is the page get_chat_log uses: the cap keeps the MOST RECENT
// turns, so offset pages backwards into older history rather than forwards from
// the beginning.
//
// The direction is the whole point of the cap. A chat's latest turns are what
// tells a reader what that chat is doing now; its first turns are how it opened,
// which is rarely why anyone reads a sibling's log. Paging with the same forward
// offset the other tools use would have made the default page the oldest turns
// and the interesting ones the ones a model had to go looking for.
func recentWindow(
	total int,
	offset int,
	limit int,
	fallback int,
	ceiling int,
) window {
	size := clampLimit(limit, fallback, ceiling)
	end := total - clamp(offset, 0, total)
	start := end - size
	if start < 0 {
		start = 0
	}
	return window{start: start, end: end, total: total}
}

// clampLimit turns a model's limit into a usable page size. A missing limit is
// zero after decoding, which is indistinguishable from an explicit 0 and means
// the same thing here — take the default — while the ceiling is what stops the
// argument from being a way to undo the cap.
func clampLimit(
	limit int,
	fallback int,
	ceiling int,
) int {
	if limit <= 0 {
		return fallback
	}
	if limit > ceiling {
		return ceiling
	}
	return limit
}

func clamp(
	v int,
	lo int,
	hi int,
) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// forwardNote is the line a forward-paginated tool leads with.
//
// It always renders, even when nothing was cut, because "Showing all 3 threads"
// and a bare list of three threads are the same bytes to a model only if it can
// already tell the list is complete — and it cannot. Stating the total every
// time is what makes the truncated case recognisable as truncated, and it costs
// about fifteen tokens.
//
// The next offset is spelled out rather than left as arithmetic: a model that
// has to compute where to resume is a model that sometimes computes it wrong and
// silently skips a page of findings.
func forwardNote(
	noun string,
	tool string,
	w window,
) string {
	if w.total == 0 {
		return ""
	}
	if w.empty() {
		return fmt.Sprintf(
			"No %s at that offset; there are %d in total. Call %s with offset below %d.\n",
			noun, w.total, tool, w.total,
		)
	}
	if w.complete() {
		return fmt.Sprintf("Showing all %d %s.\n", w.total, noun)
	}
	if w.end >= w.total {
		return fmt.Sprintf(
			"Showing %s %d-%d of %d. This is the last page.\n",
			noun, w.start+1, w.end, w.total,
		)
	}
	return fmt.Sprintf(
		"Showing %s %d-%d of %d. %d not shown — call %s with offset=%d for the next page.\n",
		noun, w.start+1, w.end, w.total, w.total-w.end, tool, w.end,
	)
}

// recentNote is forwardNote's counterpart for recentWindow, and says the two
// things a backwards page has to say that a forwards one does not: which end of
// the list was kept, and that "offset" here counts backwards from the newest
// turn. A model that assumed forward offsets would page in the wrong direction
// and conclude the older history does not exist.
func recentNote(
	noun string,
	tool string,
	w window,
) string {
	if w.total == 0 {
		return ""
	}
	if w.empty() {
		return fmt.Sprintf(
			"No %s at that offset; there are %d in total. Call %s with offset below %d.\n",
			noun, w.total, tool, w.total,
		)
	}
	if w.complete() {
		return fmt.Sprintf("Showing all %d %s, oldest first.\n", w.total, noun)
	}
	head := fmt.Sprintf(
		"Showing %s %d-%d of %d, oldest first.",
		noun, w.start+1, w.end, w.total,
	)
	if w.start == 0 {
		return head + " Nothing older exists.\n"
	}
	return head + fmt.Sprintf(
		" %d older %s not shown — call %s with offset=%d for the %d before these.\n",
		w.start, noun, tool, w.total-w.start, min(w.end-w.start, w.start),
	)
}
