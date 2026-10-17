package migrate

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
)

// Render writes the report as the text `crowbar migrate --plan` prints.
func (r Report) Render(
	w io.Writer,
) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	p := &printer{w: tw}
	r.renderHome(p)
	if len(r.Steps) == 0 {
		p.linef("\nnothing to do")
	} else {
		r.renderSteps(p)
		r.renderBackup(p)
	}
	return errors.Join(p.err, tw.Flush())
}

// printer keeps the first write error, so Render reports it once.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) linef(
	format string,
	args ...any,
) {
	if p.err != nil {
		return
	}
	_, p.err = fmt.Fprintf(p.w, format+"\n", args...)
}

func (r Report) renderHome(
	p *printer,
) {
	lock := "free"
	if r.LockHeld {
		lock = "held by another crowbar process"
	}
	layout := strconv.Itoa(r.Layout)
	if r.Layout == 0 {
		layout += " (legacy)"
	}
	p.linef("%-10s%s", "home", r.Home)
	p.linef("%-10s%s   this binary knows up to %d", "layout", layout, r.Known)
	p.linef("%-10s%s", "lock", lock)
	p.linef("%-10s%s", "last run", r.Last.describe())
}

// Each step's actions are one column block, so they align within a step.
func (r Report) renderSteps(
	p *printer,
) {
	noun := "steps"
	if len(r.Steps) == 1 {
		noun = "step"
	}
	p.linef("\n%d pending %s, in execution order", len(r.Steps), noun)
	for _, sp := range r.Steps {
		step := sp.Step
		layout := fmt.Sprintf("layout %d", step.From)
		if step.bumps() {
			layout += fmt.Sprintf(" -> %d", step.To)
		}
		p.linef("  #%d %s   %s   %s", step.Number, step.Name, step.Phase, layout)
		if sp.Plan.Empty() {
			p.linef("    (nothing to change)")
		}
		for _, a := range sp.Plan.Actions {
			p.linef("    %s\t%s\t%s", a.Kind, a.Subject, countText(a.Count))
		}
	}
}

func (r Report) renderBackup(
	p *printer,
) {
	b := r.Backup
	switch {
	case b.Kept != "":
		p.linef("\nstate backup  kept from run %s", b.Kept)
	case b.Required:
		size := uint64(b.Bytes) //nolint:gosec // G115: Bytes sums file lengths, so it is never negative
		p.linef("\nstate backup  required   %s to copy, %s free", byteSize(size), byteSize(b.FreeBytes))
	default:
		p.linef("\nstate backup  not needed")
	}
}

func countText(
	n int64,
) string {
	if n == Unknown {
		return "?"
	}
	return strconv.FormatInt(n, 10)
}

// describe is the run on one line; a nil run is none.
func (s *RunSummary) describe() string {
	if s == nil {
		return "none"
	}
	outcome, layout := s.Outcome, fmt.Sprintf("layout %d", s.LayoutFrom)
	if outcome == "" {
		outcome = "not finished"
	} else {
		layout += fmt.Sprintf(" -> %d", s.LayoutTo)
	}
	parts := []string{s.ID, outcome, layout}
	if s.Version != "" {
		parts = append(parts, "version "+s.Version)
	}
	if len(s.Counts) > 0 {
		parts = append(parts, "counts "+countsText(s.Counts))
	}
	if s.Error != "" {
		parts = append(parts, "error: "+s.Error)
	}
	return strings.Join(parts, "   ")
}

func countsText(
	c Counts,
) string {
	labels := slices.Sorted(maps.Keys(c))
	pairs := make([]string, 0, len(labels))
	for _, label := range labels {
		pairs = append(pairs, label+"="+strconv.FormatInt(c[label], 10))
	}
	return strings.Join(pairs, " ")
}

func byteSize(
	n uint64,
) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value, suffix := float64(n), ""
	for _, suffix = range []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"} {
		if value /= unit; value < unit {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, suffix)
}
