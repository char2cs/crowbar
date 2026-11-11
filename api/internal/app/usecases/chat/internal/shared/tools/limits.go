package tools

import (
	"fmt"
)

// Every list a tool renders is capped at roughly 4k tokens (~16 KB) per full page, small enough to sit
// beside every tool schema and the model's work in one turn. No cap is silent: each tool leads with a
// note giving the range, the total, and the argument that fetches the rest.
const (
	// defaultThreadPage / maxThreadPage bound one page of list_review_threads; the ceiling exists because
	// limit is model-supplied.
	defaultThreadPage = 20
	maxThreadPage     = 50

	// defaultChatLogTurns / maxChatLogTurns bound get_chat_log and HandoffWindow, so "recent
	// history" is one number. 20 turns (~ten exchanges) shows what a sibling chat is doing now.
	defaultChatLogTurns = 20
	maxChatLogTurns     = 50

	// defaultScopeFiles / maxScopeFiles bound get_review_scope's file list. A row is one short line, so
	// 100 (~1.5k tokens) covers most branches; the cap keeps a generated-code drop usable.
	defaultScopeFiles = 100
	maxScopeFiles     = 300
)

// window is the slice of a list one tool call renders, plus the total it was cut from, which the
// truncation note reports.
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

// forwardWindow is the ordinary offset/limit page. Both arguments come from a model, so both are
// clamped: never an out-of-range slice.
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

// recentWindow is get_chat_log's page: it keeps the most recent turns and offset pages backwards,
// because a chat's latest turns are what tell a reader what it is doing now.
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

// clampLimit turns a model's limit into a page size: zero or missing takes the default, and the
// ceiling stops the argument from undoing the cap.
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

// forwardNote is the line a forward-paginated tool leads with. It renders even when nothing was cut, so
// a model can tell a complete list from a truncated one, and spells out the next offset so none is
// miscomputed.
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

// recentNote is forwardNote's counterpart for recentWindow: it also says which end was kept and that
// offset counts back from the newest turn, so a model does not page the wrong way.
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
