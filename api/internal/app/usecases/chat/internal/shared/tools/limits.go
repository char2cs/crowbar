package tools

import (
	"fmt"
)

const (
	defaultThreadPage = 20
	maxThreadPage     = 50

	defaultChatLogTurns = 20
	maxChatLogTurns     = 50

	defaultScopeFiles = 100
	maxScopeFiles     = 300
)

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
