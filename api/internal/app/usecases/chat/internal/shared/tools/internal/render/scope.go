package render

import (
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

const scopeRangesLegend = "Changed line ranges follow each file, indented: right is this branch, " +
	"left the base revision, both ends inclusive. post_review_comment needs its whole anchor " +
	"inside ONE range, on the side it names.\n"

const scopeNoRangesLine = "    (no changed lines in this diff — this file cannot carry an anchored comment)\n"

func RenderScope(
	base string,
	files []gitdomain.ReviewFileSummary,
	outline []gitdomain.FileOutline,
	first int,
	note string,
) string {
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
	if len(files) == 0 && note == "" {
		b.WriteString("No changed files.\n")
	}
	if len(files) == 0 {
		return b.String()
	}
	if geom.shown > 0 {
		b.WriteString(scopeRangesLegend)
	}
	b.WriteString("status  +adds  -dels  path\n")
	b.WriteString(rows.String())
	return b.String()
}

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
	if f.IsPartial {
		line += "  (this file has more hunks than the review's outline collected; " +
			"ranges past the last one are unknown)"
	}
	return line + "\n", len(right) + len(left)
}

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
