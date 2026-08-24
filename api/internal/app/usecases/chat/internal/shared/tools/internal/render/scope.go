package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// scopeRangesLegend states, once per reply, how to read the indented range lines
// and what the tool that consumes them requires.
//
// It is spelled out rather than left implicit because the numbers alone are
// ambiguous in the two ways that matter: which side's numbering they are in, and
// whether an anchor may span two of them. A model that assumes it may span is
// refused, which is the exact rejection this listing exists to make unreachable.
const scopeRangesLegend = "Changed line ranges follow each file, indented: right is this branch, " +
	"left the base revision, both ends inclusive. post_review_comment needs its whole anchor " +
	"inside ONE range, on the side it names.\n"

// scopeNoRangesLine marks a file no comment can be anchored to.
//
// It is stated rather than left blank because a file row with nothing under it
// reads as a file whose ranges simply were not printed. This case is real and it
// is permanent: git reports no line-level changes for a binary file, and an
// untracked file is in the review's file list (status merges it in) but not in
// its diff at all, so post_review_comment will refuse every anchor on it.
const scopeNoRangesLine = "    (no changed lines in this diff — this file cannot carry an anchored comment)\n"

// RenderScope reports the base ref, the page of changed files it was handed, and
// each file's changed line ranges.
//
// The ranges are the difference between a list a model can act on and one it has
// to guess against: post_review_comment accepts an anchor only inside a changed
// hunk, and until this listing carried them the only tool that would tell a model
// where those were was the REJECTION it got for guessing wrong. They come from
// the same outline that validator reads, resolved in the same call (see
// gitdomain.ReviewScope), so a range printed here is a range that tool accepts.
//
// note carries whatever get_review_scope's pagination has to say and is emitted
// FIRST, above the base line: a model reads top-down and may stop reading a long
// file list early, so a truncation statement placed under the rows it qualifies
// is a statement that arrives too late to change what the model concludes. It is
// empty when there is nothing to say. The geometry's own budget note joins it
// there, for the same reason and in the same place.
//
// first is the absolute index of this page's first file, and is needed for
// exactly one sentence: when the range budget runs out mid-page, the note has to
// name the offset that fetches the ranges of the files it stopped at.
func RenderScope(
	base string,
	files []gitdomain.ReviewFileSummary,
	outline []gitdomain.FileOutline,
	first int,
	note string,
) string {
	// The rows are built before anything is written, because how much geometry
	// the page could afford is only known once every row has been walked — and
	// the sentence saying so belongs at the top, with the other notes.
	geom := newScopeGeometry(outline)
	var rows strings.Builder
	for i, f := range files {
		fmt.Fprintf(&rows, "%s  +%d  -%d  %s\n", f.Status, f.Additions, f.Deletions, f.Path)
		geom.write(&rows, f, i)
	}

	var b strings.Builder
	b.WriteString(note)
	b.WriteString(geom.note(first))
	fmt.Fprintf(&b, "This review covers everything on this branch since %s.\n", base)
	// "No changed files." is the answer for a branch with nothing on it. An empty
	// PAGE — a caller that paged past the last file — is a different answer, and
	// note has already given it; repeating this line there would tell a model the
	// branch is clean when it is only looking past the end.
	if len(files) == 0 && note == "" {
		b.WriteString("No changed files.\n")
	}
	if len(files) == 0 {
		return b.String()
	}
	// The legend costs about forty tokens and is pure overhead on a page that
	// printed no ranges at all — a review of nothing but binaries, or a page past
	// the budget — so it is only written when there is something to read with it.
	if geom.shown > 0 {
		b.WriteString(scopeRangesLegend)
	}
	b.WriteString("status  +adds  -dels  path\n")
	b.WriteString(rows.String())
	return b.String()
}

// scopeGeometry renders one page's changed line ranges and keeps the page's range
// budget.
//
// The budget is per PAGE rather than per file because the two bound different
// failures. A file's own cap stops one pathological file — a generated file
// rewritten line by line carries up to MaxOutlineHunksPerFile hunks — from
// spending the whole reply; the page's stops three hundred ordinary files from
// doing it between them. See MaxScopeRanges.
//
// firstSilent is the page-relative index of the first file the budget could not
// afford, which is what the note turns into the offset that fetches it.
type scopeGeometry struct {
	index       map[string]gitdomain.FileOutline
	budget      int
	shown       int
	silent      int
	firstSilent int
}

func newScopeGeometry(
	outline []gitdomain.FileOutline,
) *scopeGeometry {
	return &scopeGeometry{
		index:       outlineIndex(outline),
		budget:      MaxScopeRanges,
		firstSilent: -1,
	}
}

// outlineIndex keys the geometry by BOTH of a file's names. A rename is reported
// under its new path by the file summary and carries both in the outline, and an
// index on one name alone would leave the renamed file looking like a file with
// no changed lines — which is the one rendering that tells a model not to comment
// on it.
func outlineIndex(
	outline []gitdomain.FileOutline,
) map[string]gitdomain.FileOutline {
	index := make(map[string]gitdomain.FileOutline, len(outline))
	for _, f := range outline {
		index[f.Path] = f
		if f.OldPath != "" {
			index[f.OldPath] = f
		}
	}
	return index
}

// write emits one file's range line, or nothing when the page's budget is spent.
//
// A file with no geometry at all costs no budget: its line is a fixed sentence,
// not a list, and charging for it would let a review full of binaries crowd out
// the ranges of the files that have some.
func (g *scopeGeometry) write(
	b *strings.Builder,
	f gitdomain.ReviewFileSummary,
	index int,
) {
	file, ok := g.index[f.Path]
	if !ok {
		file, ok = g.index[f.OldPath]
	}
	if !ok || !hasAnyRange(file) {
		b.WriteString(scopeNoRangesLine)
		return
	}
	if g.budget <= 0 {
		if g.firstSilent < 0 {
			g.firstSilent = index
		}
		g.silent++
		return
	}
	line, spent := scopeRangeLine(file, g.budget)
	b.WriteString(line)
	g.budget -= spent
	g.shown++
}

// note states what the budget could not afford, and names the offset that
// fetches it — the same recovery shape every other cap on this surface gives,
// because a page whose later files silently carry no ranges reads as a page whose
// later files have no changed lines.
func (g *scopeGeometry) note(
	first int,
) string {
	if g.silent == 0 {
		return ""
	}
	return fmt.Sprintf(
		"Changed line ranges are listed for the first %d files below; the last %d have none "+
			"listed — call get_review_scope with offset=%d for theirs.\n",
		g.shown, g.silent, first+g.firstSilent,
	)
}

func hasAnyRange(
	f gitdomain.FileOutline,
) bool {
	return len(RangesOnSide(f.Hunks, domain.ReviewSideRight)) > 0 ||
		len(RangesOnSide(f.Hunks, domain.ReviewSideLeft)) > 0
}

// scopeRangeLine renders one file's ranges on both sides and reports how many it
// spent from the page's budget.
//
// Both sides are printed, not just the branch's, because leaving the base side
// out is worse than merely incomplete: a model commenting on removed code is told
// to use side=left, and left line numbers diverge from right ones by every
// insertion above them. Given only right-side numbers it would anchor with them
// on the left, where they can land inside a DIFFERENT hunk and be accepted — a
// comment stored against the wrong lines, which is the failure no rejection
// message can catch.
//
// The right side is served first when the allowance is tight, because it is the
// side almost every review comment is on (see the tool's own `side` description).
func scopeRangeLine(
	f gitdomain.FileOutline,
	allowance int,
) (string, int) {
	right, rightHidden := cappedRanges(f.Hunks, domain.ReviewSideRight, allowance)
	left, leftHidden := cappedRanges(f.Hunks, domain.ReviewSideLeft, allowance-len(right))

	groups := make([]string, 0, 2)
	for _, g := range []string{
		sideRangeGroup("right", right, rightHidden),
		sideRangeGroup("left", left, leftHidden),
	} {
		if g != "" {
			groups = append(groups, g)
		}
	}
	line := "    " + strings.Join(groups, "  ")
	// A partial outline is a lower bound the model must not read as exhaustive:
	// its file has hunks the daemon itself never collected, so "the ranges are X"
	// would tell it the line it wanted is not in the diff when it may well be.
	if f.IsPartial {
		line += "  (this file has more hunks than the review's outline collected; " +
			"ranges past the last one are unknown)"
	}
	return line + "\n", len(right) + len(left)
}

// cappedRanges keeps the leading ranges of one side, bounded by both the file's
// own cap and whatever the page can still afford, and reports how many it left
// out.
//
// LEADING, not nearest — unlike a rejection's list, which windows around the
// anchor the model just tried. There is no anchor here yet: this is the listing a
// model reads before it has chosen one, and it reads a file from the top.
func cappedRanges(
	hunks []gitdomain.HunkShape,
	side domain.ReviewSide,
	allowance int,
) ([]HunkRange, int) {
	all := RangesOnSide(hunks, side)
	keep := min(MaxScopeRangesPerFile, max(allowance, 0))
	if len(all) <= keep {
		return all, 0
	}
	return all[:keep], len(all) - keep
}

// sideRangeGroup renders one side's contribution, and is empty for a side the
// file does not touch at all — a pure addition has no left side, and printing
// "left" with nothing after it would read as a side whose ranges were withheld.
func sideRangeGroup(
	label string,
	shown []HunkRange,
	hidden int,
) string {
	switch {
	case len(shown) == 0 && hidden == 0:
		return ""
	case len(shown) == 0:
		return fmt.Sprintf("%s %d ranges not shown", label, hidden)
	case hidden == 0:
		return label + " " + RenderRanges(shown)
	}
	return fmt.Sprintf("%s %s (+%d more)", label, RenderRanges(shown), hidden)
}
