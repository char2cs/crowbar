package render

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// The anchor half: whether a review comment may be attached where the agent asked,
// and what it is told when it may not.
//
// It lives beside the renderers rather than in a package of its own because the two
// share the diff's geometry — the same hunk ranges that decide an anchor is invalid
// are the ones a scope listing prints — and a refusal here IS agent-facing text. A
// verdict a model cannot read is a verdict it cannot act on.

// MaxBodyChars caps a body on the way IN — the finding a model posts. It is the
// renderer's because the same call that places a comment is the one that
// decides its body is postable.
const MaxBodyChars = 4000

const (
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
)

// checkBody is the one gate every free-text body a model WRITES passes through:
// it must say something, and it must be small enough to store.
//
// The upper bound is the half that did not exist. maxMessageBodyChars and
// maxTurnBodyChars bound what is rendered BACK; nothing bounded what went in, so
// a body was unbounded in the store and — because a reply emits the whole thread
// aggregate as its event payload — re-serialised on every later message of that
// thread. See MaxBodyChars.
//
// The refusal names the length AND the limit, because the model's only recovery
// is to shorten and retry, and it cannot do that against an error that merely
// says "too long". Runes are counted for the same reason truncateBody counts
// them: the limit is about how much text this is, not how many bytes UTF-8 spent
// on it.
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

// validateAnchor rejects an anchor that does not land in a hunk of the CURRENT
// review rather than storing it: a floating comment is worse than no comment,
// because the user sees a finding with no code next to it and cannot tell what it
// refers to.
//
// The side selects which pair of the hunk's four numbers applies. right is the
// branch (NewStart/NewLines), left the base revision (OldStart/OldLines) — a
// deleted line only exists on the left, and the two numberings diverge by every
// insertion above them, so validating against the wrong pair would accept anchors
// that render nowhere.
//
// A file matches on either path because a rename carries both: Path is the new
// name the review addresses it by, OldPath the name it had on the base side.
//
// A binary file is rejected by the hunk loop finding nothing to match: git emits no
// `@@` headers for one, so its Hunks slice is empty.
//
// A file whose outline is partial (its hunk count ran past the collection cap) can
// still refuse a legitimate anchor past the cap. That is the safe direction: the
// model is told to re-read the scope, rather than the user being shown a comment
// against nothing.
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
		// The refusal is built from the file's own geometry rather than written as
		// a fixed sentence, because the model has to recover from it and the only
		// thing it can recover with is where the legal anchors are. See
		// anchorRejection.
		return anchorRejection(f, path, start, end, side)
	}
	return fmt.Errorf(
		"agenttools: %s is not part of this review; call get_review_scope for the changed files",
		path,
	)
}

// anchorInAnyHunk requires the WHOLE range to fall inside one hunk. lo+span-1 is
// the hunk's last line, so a hunk at NewStart 40 spanning 10 lines covers 40..49
// inclusive: 39 and 50 are outside it. A side that is absent (`@@ -1,2 +0,0 @@`
// gives span 0) has an empty range and matches nothing, since checkAnchorRange has
// already established start <= end.
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
	ranges := RangesOnSide(file.Hunks, side)
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
		start, end, path, side, path, side, RenderRanges(shown), rangesCaveat(hidden, file.IsPartial),
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
	elsewhere := RangesOnSide(file.Hunks, other)
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
		start, end, path, side, other, RenderRanges(shown), rangesCaveat(hidden, file.IsPartial), other,
	)
}

// RangesOnSide reduces the hunks to the ranges a comment may anchor inside on one
// side, in the same order git emitted them.
//
// A span of zero is dropped rather than rendered: `@@ -1,2 +0,0 @@` gives the new
// side start 0 and span 0, and "0--1" is not a range anybody can anchor to.
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

// nearestRanges picks the ranges to name, and reports how many it left out.
//
// It keeps a contiguous window around the range CLOSEST to what the model tried,
// not the first few in the file. A file may carry up to a thousand hunks, and a
// model that anchored near the bottom of one is not helped by the ranges at the
// top; a window keeps the answer near the model's own intent and keeps the list in
// file order with no second sort.
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
