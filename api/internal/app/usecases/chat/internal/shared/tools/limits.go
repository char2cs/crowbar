package tools

import (
	"fmt"
)

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
