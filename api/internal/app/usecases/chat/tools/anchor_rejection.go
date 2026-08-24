package tools

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// hunkRange is one changed range of a file on ONE side, in that side's own line
// numbering, inclusive at both ends — the form a model has to anchor inside.
type hunkRange struct {
	lo int
	hi int
}

func (r hunkRange) String() string {
	return strconv.Itoa(r.lo) + "-" + strconv.Itoa(r.hi)
}

// anchorRejection is the message a model reads when its anchor was refused, and
// the whole reason it is built rather than written as one constant string: a
// rejection is a recovery instruction, and the only recovery instruction worth
// giving is the LEGAL MOVE.
//
// The rule itself does not move. The whole range must sit inside ONE hunk, and it
// fails closed, because a comment that floats off the diff shows the user a
// finding with no code beside it. What changed is that the message now says what
// the legal anchors ARE — the server is holding the file's hunk geometry at this
// exact point, and withholding it left the model guessing at the same numbers
// again.
//
// The message it replaced said "are not in any changed hunk ...; call
// get_review_scope and anchor inside a changed range", which failed a model twice
// over. It was wrong about the commonest rejection — an anchor spanning two hunks
// has BOTH endpoints in a changed hunk, so a model told none of it was changed
// moves the anchor in the wrong direction — and the recovery move it named is a
// dead end, because get_review_scope reports a status, two counts and a path per
// file and no line numbers whatsoever. get_review_scope is still named here, for
// the question it can actually answer: which files the review covers.
func anchorRejection(
	file gitdomain.FileOutline,
	path string,
	start int,
	end int,
	side domain.ReviewSide,
) error {
	ranges := rangesOnSide(file.Hunks, side)
	if len(ranges) == 0 {
		return sidelessAnchorRejection(file, path, start, end, side)
	}
	shown, hidden := nearestRanges(ranges, start, end)
	//nolint:staticcheck // ST1005: this is prose for a MODEL recovering from a rejection,
	// not a clause a Go caller wraps with %w and re-cases — it is never wrapped further
	// (see validateAnchor's call site), so the terminal period ends a real sentence
	// instead of dangling mid-thought. See the doc above for why the wording is fixed.
	return fmt.Errorf(
		"agenttools: post_review_comment: lines %d-%d of %s are not inside a single changed hunk "+
			"on the %s side. A comment must sit entirely within ONE changed range — a range that "+
			"spans two hunks, or that covers the unchanged lines between them, is refused. The "+
			"changed ranges of %s on the %s side are: %s%s. Re-anchor inside one of them; "+
			"get_review_scope lists the files this review covers.",
		start, end, path, side, path, side, renderRanges(shown), rangesCaveat(hidden, file.IsPartial),
	)
}

// sidelessAnchorRejection answers the two cases where the side the model chose has
// no changed lines at all, and they need different advice: a file that changed
// only on the OTHER side (a deletion, whose lines exist only on the left) can be
// commented on by switching sides, and a file with no line-level diff on either
// side (a binary) can never be commented on and the model should stop trying.
//
// Telling the two apart matters because the first is one argument away from
// success and the second is not, and the old message gave both the same advice.
func sidelessAnchorRejection(
	file gitdomain.FileOutline,
	path string,
	start int,
	end int,
	side domain.ReviewSide,
) error {
	other := otherSide(side)
	elsewhere := rangesOnSide(file.Hunks, other)
	if len(elsewhere) == 0 {
		//nolint:staticcheck // ST1005: prose for a model, not a wrapped Go clause — same
		// rationale as anchorRejection above.
		return fmt.Errorf(
			"agenttools: post_review_comment: lines %d-%d of %s cannot be anchored: git reports no "+
				"line-level changes for this file on either side, which is what a binary file looks "+
				"like. It cannot carry an anchored comment; get_review_scope lists the files that can.",
			start, end, path,
		)
	}
	shown, hidden := nearestRanges(elsewhere, start, end)
	//nolint:staticcheck // ST1005: prose for a model, not a wrapped Go clause — same
	// rationale as anchorRejection above.
	return fmt.Errorf(
		"agenttools: post_review_comment: lines %d-%d of %s cannot be anchored on the %s side: that "+
			"side of the file has no changed lines at all. It changed only on the %s side, where the "+
			"ranges are: %s%s. Re-anchor with side=%q.",
		start, end, path, side, other, renderRanges(shown), rangesCaveat(hidden, file.IsPartial), other,
	)
}

// rangesOnSide reduces the hunks to the ranges a comment may anchor inside on one
// side, in the same order git emitted them.
//
// A span of zero is dropped rather than rendered: `@@ -1,2 +0,0 @@` gives the new
// side start 0 and span 0, and "0--1" is not a range anybody can anchor to.
func rangesOnSide(
	hunks []gitdomain.HunkShape,
	side domain.ReviewSide,
) []hunkRange {
	out := make([]hunkRange, 0, len(hunks))
	for _, h := range hunks {
		lo, span := h.NewStart, h.NewLines
		if side == domain.ReviewSideLeft {
			lo, span = h.OldStart, h.OldLines
		}
		if span <= 0 {
			continue
		}
		out = append(out, hunkRange{lo: lo, hi: lo + span - 1})
	}
	return out
}

// nearestRanges picks the ranges to name, and reports how many it left out.
//
// It keeps a contiguous window around the range CLOSEST to what the model tried,
// not the first few in the file. A file may carry up to a thousand hunks, and a
// model that anchored near the bottom of one is not helped by the ranges at the
// top; a window keeps the answer near the model's own intent and keeps the list in
// file order with no second sort.
func nearestRanges(
	ranges []hunkRange,
	start int,
	end int,
) ([]hunkRange, int) {
	if len(ranges) <= maxAnchorRangesShown {
		return ranges, 0
	}
	lo := closestRange(ranges, start, end) - maxAnchorRangesShown/2
	if lo < 0 {
		lo = 0
	}
	if lo+maxAnchorRangesShown > len(ranges) {
		lo = len(ranges) - maxAnchorRangesShown
	}
	return ranges[lo : lo+maxAnchorRangesShown], len(ranges) - maxAnchorRangesShown
}

// closestRange is the index of the range nearest the attempted anchor, measured by
// the gap between them — zero when they overlap, which is the case for every
// endpoint of an anchor that spanned two hunks.
func closestRange(
	ranges []hunkRange,
	start int,
	end int,
) int {
	best, bestGap := 0, -1
	for i, r := range ranges {
		gap := rangeGap(r, start, end)
		if bestGap < 0 || gap < bestGap {
			best, bestGap = i, gap
		}
	}
	return best
}

func rangeGap(
	r hunkRange,
	start int,
	end int,
) int {
	if start > r.hi {
		return start - r.hi
	}
	if end < r.lo {
		return r.lo - end
	}
	return 0
}

func renderRanges(
	ranges []hunkRange,
) string {
	out := make([]string, 0, len(ranges))
	for _, r := range ranges {
		out = append(out, r.String())
	}
	return strings.Join(out, ", ")
}

// rangesCaveat states what the list does not cover, for the same reason every
// other cap on this surface states it: a model reads a short list as the whole
// list and then concludes the range it wanted does not exist.
//
// The two caveats are independent. hidden counts ranges this message chose not to
// print; partial marks a file whose hunk collection was capped upstream, so ranges
// exist that the daemon itself never saw.
func rangesCaveat(
	hidden int,
	partial bool,
) string {
	var b strings.Builder
	if hidden > 0 {
		fmt.Fprintf(&b, " (%d further changed ranges in this file are not listed)", hidden)
	}
	if partial {
		b.WriteString(" (this file has more hunks than the review's outline collected, so ranges below the last one listed are unknown)")
	}
	return b.String()
}

func otherSide(
	side domain.ReviewSide,
) domain.ReviewSide {
	if side == domain.ReviewSideLeft {
		return domain.ReviewSideRight
	}
	return domain.ReviewSideLeft
}
