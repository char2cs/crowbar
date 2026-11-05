package render

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// The anchor half: whether a review comment may sit where the agent asked, and what it is told if not.
// It lives beside the renderers because both share the diff's hunk geometry and a refusal is agent text.

// MaxBodyChars caps a body a model writes. Stored bodies are re-serialised in every later event of
// their thread, so unbounded input would grow without limit.
const MaxBodyChars = 4000

const (
	// maxAnchorRangesShown bounds how many changed ranges a rejected anchor is told about. A file may
	// have a thousand hunks; eight is enough because the ranges shown are those nearest the attempt.
	maxAnchorRangesShown = 8
)

// CheckBody gates every free-text body a model writes: non-empty and at most MaxBodyChars runes. The
// refusal names the length and the limit because shortening and retrying is the model's only recovery.
func CheckBody(tool, body string) error {
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("agenttools: %s: body must not be empty", tool)
	}
	if n := utf8.RuneCountInString(body); n > MaxBodyChars {
		return fmt.Errorf(
			"agenttools: %s: body is %d characters, over the %d-character limit; "+
				"shorten it and call again",
			tool, n, MaxBodyChars,
		)
	}
	return nil
}

func ParseSide(side string) (domain.ReviewSide, error) {
	switch domain.ReviewSide(side) {
	case domain.ReviewSideLeft:
		return domain.ReviewSideLeft, nil
	case domain.ReviewSideRight:
		return domain.ReviewSideRight, nil
	}
	return "", fmt.Errorf("agenttools: post_review_comment: side must be \"left\" or \"right\", got %q", side)
}

func CheckRange(start, end int) error {
	if start < 1 || end < start {
		return fmt.Errorf(
			"agenttools: post_review_comment: startLine %d and endLine %d are not a line range; "+
				"lines are 1-based and endLine must not precede startLine",
			start, end,
		)
	}
	return nil
}

// Validate rejects an anchor outside every hunk of the current review on its side (right = branch,
// left = base): a floating comment shows the user a finding with no code beside it. A file matches on
// Path or OldPath (renames); a partial outline may refuse a valid anchor, which is the safe direction.
func Validate(
	outline []gitdomain.FileOutline,
	path string,
	start, end int,
	side domain.ReviewSide,
) error {
	for _, f := range outline {
		if f.Path != path && f.OldPath != path {
			continue
		}
		if anchorInAnyHunk(f.Hunks, start, end, side) {
			return nil
		}
		return anchorRejection(f, path, start, end, side)
	}
	return fmt.Errorf(
		"agenttools: %s is not part of this review; call get_review_scope for the changed files",
		path,
	)
}

// anchorInAnyHunk requires the whole range inside one hunk, inclusive at both ends. An absent side
// (span 0) matches nothing, since CheckRange already established start <= end.
func anchorInAnyHunk(
	hunks []gitdomain.HunkShape,
	start, end int,
	side domain.ReviewSide,
) bool {
	for _, h := range hunks {
		lo, span := h.NewStart, h.NewLines
		if side == domain.ReviewSideLeft {
			lo, span = h.OldStart, h.OldLines
		}
		if start >= lo && end <= lo+span-1 {
			return true
		}
	}
	return false
}

// HunkRange is one changed range of a file on ONE side, in that side's own line
// numbering, inclusive at both ends — the form a model has to anchor inside.
type HunkRange struct {
	lo int
	hi int
}

func (r HunkRange) String() string {
	return strconv.Itoa(r.lo) + "-" + strconv.Itoa(r.hi)
}

// anchorRejection is built from the file's own hunk geometry because a rejection is a recovery
// instruction: it names the legal ranges, since get_review_scope reports no line numbers and the
// commonest refusal (a range spanning two hunks) has both endpoints inside changed lines.
func anchorRejection(
	file gitdomain.FileOutline,
	path string,
	start int,
	end int,
	side domain.ReviewSide,
) error {
	ranges := RangesOnSide(file.Hunks, side)
	if len(ranges) == 0 {
		return sidelessAnchorRejection(file, path, start, end, side)
	}
	shown, hidden := nearestRanges(ranges, start, end)
	//nolint:staticcheck // ST1005: prose for a model, never wrapped further, so it ends in a period.
	return fmt.Errorf(
		"agenttools: post_review_comment: lines %d-%d of %s are not inside a single changed hunk "+
			"on the %s side. A comment must sit entirely within ONE changed range — a range that "+
			"spans two hunks, or that covers the unchanged lines between them, is refused. The "+
			"changed ranges of %s on the %s side are: %s%s. Re-anchor inside one of them; "+
			"get_review_scope lists the files this review covers.",
		start, end, path, side, path, side, RenderRanges(shown), rangesCaveat(hidden, file.IsPartial),
	)
}

// sidelessAnchorRejection answers a side with no changed lines: a file changed only on the other side
// (a deletion) is one argument away from success, while one with no line diff at all (a binary) never
// takes an anchored comment.
func sidelessAnchorRejection(
	file gitdomain.FileOutline,
	path string,
	start int,
	end int,
	side domain.ReviewSide,
) error {
	other := otherSide(side)
	elsewhere := RangesOnSide(file.Hunks, other)
	if len(elsewhere) == 0 {
		//nolint:staticcheck // ST1005: prose for a model, never wrapped further.
		return fmt.Errorf(
			"agenttools: post_review_comment: lines %d-%d of %s cannot be anchored: git reports no "+
				"line-level changes for this file on either side, which is what a binary file looks "+
				"like. It cannot carry an anchored comment; get_review_scope lists the files that can.",
			start, end, path,
		)
	}
	shown, hidden := nearestRanges(elsewhere, start, end)
	//nolint:staticcheck // ST1005: prose for a model, never wrapped further.
	return fmt.Errorf(
		"agenttools: post_review_comment: lines %d-%d of %s cannot be anchored on the %s side: that "+
			"side of the file has no changed lines at all. It changed only on the %s side, where the "+
			"ranges are: %s%s. Re-anchor with side=%q.",
		start, end, path, side, other, RenderRanges(shown), rangesCaveat(hidden, file.IsPartial), other,
	)
}

// RangesOnSide reduces the hunks to the ranges a comment may anchor inside on one side, in git order.
// A zero span (`@@ -1,2 +0,0 @@`) is dropped: "0--1" is not an anchorable range.
func RangesOnSide(
	hunks []gitdomain.HunkShape,
	side domain.ReviewSide,
) []HunkRange {
	out := make([]HunkRange, 0, len(hunks))
	for _, h := range hunks {
		lo, span := h.NewStart, h.NewLines
		if side == domain.ReviewSideLeft {
			lo, span = h.OldStart, h.OldLines
		}
		if span <= 0 {
			continue
		}
		out = append(out, HunkRange{lo: lo, hi: lo + span - 1})
	}
	return out
}

// nearestRanges picks a contiguous window around the range closest to the attempt, and reports how
// many it left out. A window keeps the list near the model's intent and in file order without a sort.
func nearestRanges(
	ranges []HunkRange,
	start int,
	end int,
) ([]HunkRange, int) {
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
	ranges []HunkRange,
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
	r HunkRange,
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

func RenderRanges(
	ranges []HunkRange,
) string {
	out := make([]string, 0, len(ranges))
	for _, r := range ranges {
		out = append(out, r.String())
	}
	return strings.Join(out, ", ")
}

// rangesCaveat states what the list omits, since a model reads a short list as complete. hidden counts
// ranges not printed; partial marks a file whose hunks were capped upstream, so some were never seen.
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
