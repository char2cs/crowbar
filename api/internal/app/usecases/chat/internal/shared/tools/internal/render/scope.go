package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// scopeRangesLegend says, once per reply, which side's numbering the ranges use and that an anchor may
// not span two of them; the bare numbers are ambiguous on both points.
const scopeRangesLegend = "Changed line ranges follow each file, indented: right is this branch, " +
	"left the base revision, both ends inclusive. post_review_comment needs its whole anchor " +
	"inside ONE range, on the side it names.\n"

// scopeNoRangesLine marks a file no comment can anchor to (a binary, or an untracked file that is listed
// but absent from the diff); left blank, the row would read as ranges merely not printed.
const scopeNoRangesLine = "    (no changed lines in this diff — this file cannot carry an anchored comment)\n"

// RenderScope reports the base ref, a page of changed files and their changed ranges, taken from the
// outline Validate checks so a printed range is one post_review_comment accepts. note (pagination) is
// written first because a model may stop reading early; first is the page's absolute offset.
func RenderScope(
	base string,
	files []gitdomain.ReviewFileSummary,
	outline []gitdomain.FileOutline,
	first int,
	note string,
) string {
	// Rows are built first: the budget note they produce goes at the top.
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
	// An empty page past the end is explained by note; only a clean branch says "No changed files.".
	if len(files) == 0 && note == "" {
		b.WriteString("No changed files.\n")
	}
	if len(files) == 0 {
		return b.String()
	}
	// The legend is overhead on a page that printed no ranges.
	if geom.shown > 0 {
		b.WriteString(scopeRangesLegend)
	}
	b.WriteString("status  +adds  -dels  path\n")
	b.WriteString(rows.String())
	return b.String()
}

// scopeGeometry renders one page's changed ranges against the page budget (MaxScopeRanges).
// firstSilent is the page-relative index of the first file the budget could not afford.
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

// outlineIndex keys the geometry by both of a file's names, so a rename is not rendered as a file with
// no changed lines.
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

// write emits one file's range line, or nothing when the page's budget is spent. A file with no
// geometry costs no budget, so binaries cannot crowd out files that have ranges.
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

// note states what the budget could not afford and the offset that fetches it; silently missing ranges
// would read as files with no changed lines.
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

// scopeRangeLine renders one file's ranges on both sides and reports how many it spent. Left numbers are
// printed too: right-side numbers used on the left can land in a different hunk and be wrongly accepted.
// The right side is served first when the allowance is tight, as almost every comment is on it.
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
	// A partial outline is a lower bound: the file has hunks the daemon never collected.
	if f.IsPartial {
		line += "  (this file has more hunks than the review's outline collected; " +
			"ranges past the last one are unknown)"
	}
	return line + "\n", len(right) + len(left)
}

// cappedRanges keeps the leading ranges of one side within the file cap and the page allowance, and
// reports how many it left out. Leading, not nearest: no anchor has been chosen yet.
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

// sideRangeGroup renders one side's contribution; it is empty for a side the file does not touch, so a
// bare label never reads as withheld ranges.
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
