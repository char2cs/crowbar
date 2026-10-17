package migrate

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
)

var stepName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Catalog is a set of steps whose numbering, phases and layout chain have been
// checked. It cannot be built any other way, so every later reader can rely on
// the chain being contiguous from layout 0.
type Catalog struct {
	steps []Step
	known int
}

// NewCatalog sorts steps by number and refuses a set that is not coherent: bump
// steps must form one chain from layout 0, in-layout steps must precede the bump
// out of their layout, and post-open steps must sit at the final layout.
func NewCatalog(
	steps ...Step,
) (*Catalog, error) {
	sorted := slices.Clone(steps)
	slices.SortStableFunc(sorted, func(a, b Step) int {
		return cmp.Compare(a.Number, b.Number)
	})
	if err := checkSteps(sorted); err != nil {
		return nil, err
	}
	known, err := bumpChain(sorted)
	if err != nil {
		return nil, err
	}
	if err := checkPlacement(sorted, known); err != nil {
		return nil, err
	}
	return &Catalog{steps: sorted, known: known}, nil
}

// Known is the layout version this catalog migrates to: the end of the bump
// chain, or 0 when no step bumps the layout.
func (c *Catalog) Known() int {
	return c.known
}

// order lists the steps still to run from a home at layout; applied reports
// whether an in-layout or post-open step already completed (bump steps ignore
// it). Requires 0 <= layout <= Known().
func (c *Catalog) order(
	layout int,
	applied func(Step) bool,
) []Step {
	var out []Step
	for at := layout; at <= c.known; at++ {
		out = c.appendPreOpen(out, at, applied)
	}
	for _, s := range c.steps {
		if s.Phase == PostOpen && !applied(s) {
			out = append(out, s)
		}
	}
	return out
}

func (c *Catalog) appendPreOpen(
	out []Step,
	layout int,
	applied func(Step) bool,
) []Step {
	var bump []Step
	for _, s := range c.steps {
		if s.Phase != PreOpen || s.From != layout {
			continue
		}
		switch {
		case s.bumps():
			bump = append(bump, s)
		case !applied(s):
			out = append(out, s)
		}
	}
	return append(out, bump...)
}

func checkSteps(
	sorted []Step,
) error {
	names := make(map[string]bool, len(sorted))
	for i, s := range sorted {
		if fault := stepFault(s); fault != "" {
			return stepError(s, fault)
		}
		if i > 0 && sorted[i-1].Number == s.Number {
			return stepError(s, "duplicate step number")
		}
		if names[s.Name] {
			return stepError(s, "duplicate step name")
		}
		names[s.Name] = true
	}
	return nil
}

func stepFault(
	s Step,
) string {
	switch {
	case s.Number < 1:
		return "step number must be >= 1"
	case !stepName.MatchString(s.Name):
		return "invalid step name"
	case s.Phase != PreOpen && s.Phase != PostOpen:
		return fmt.Sprintf("invalid phase %d", uint8(s.Phase))
	case s.From < 0 || (s.To != s.From && !s.bumps()):
		return fmt.Sprintf("invalid layout range %d->%d", s.From, s.To)
	case s.Phase == PostOpen && s.bumps():
		return "only pre-open steps may bump the layout"
	case s.Plan == nil:
		return "missing plan"
	default:
		return ""
	}
}

// bumpChain returns the layout the bump steps reach. The From values must be
// exactly 0..n-1, each once, so n is the known layout.
func bumpChain(
	sorted []Step,
) (int, error) {
	from := make(map[int]bool)
	for _, s := range sorted {
		if !s.bumps() {
			continue
		}
		if from[s.From] {
			return 0, stepError(s, fmt.Sprintf("more than one bump from layout %d", s.From))
		}
		from[s.From] = true
	}
	known := 0
	for from[known] {
		known++
	}
	if known != len(from) {
		return 0, fmt.Errorf("migrate: catalog: gap in layout chain: no bump from layout %d", known)
	}
	return known, nil
}

func checkPlacement(
	sorted []Step,
	known int,
) error {
	bumpNumber := make(map[int]int, known)
	for _, s := range sorted {
		if s.bumps() {
			bumpNumber[s.From] = s.Number
		}
	}
	for _, s := range sorted {
		if s.bumps() {
			continue
		}
		if fault := placementFault(s, known, bumpNumber); fault != "" {
			return stepError(s, fault)
		}
	}
	return nil
}

func placementFault(
	s Step,
	known int,
	bumpNumber map[int]int,
) string {
	switch {
	case s.Phase == PostOpen && s.From != known:
		return fmt.Sprintf("post-open step must sit at the known layout %d, not %d", known, s.From)
	case s.Phase == PostOpen:
		return ""
	case s.From > known:
		return fmt.Sprintf("in-layout step at layout %d is beyond the known layout %d", s.From, known)
	case s.From < known && s.Number > bumpNumber[s.From]:
		return fmt.Sprintf("in-layout step at layout %d must precede the bump (step %d)", s.From, bumpNumber[s.From])
	default:
		return ""
	}
}

func stepError(
	s Step,
	fault string,
) error {
	return fmt.Errorf("migrate: catalog: step %d %q: %s", s.Number, s.Name, fault)
}
