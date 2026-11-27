package render

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

const MaxBodyChars = 4000

const (
	maxAnchorRangesShown = 8
)

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

type HunkRange struct {
	lo int
	hi int
}

func (r HunkRange) String() string {
	return strconv.Itoa(r.lo) + "-" + strconv.Itoa(r.hi)
}

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
