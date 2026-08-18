package rules

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type terminalNotices struct{}

func (terminalNotices) Name() string { return "terminal_notices" }

// Check is stricter than terminalPrompts, and the extra strictness is the point.
//
// A prompt needle only ever raises a banner, so an unidentified one can honestly
// report "something is up" and a descriptor may leave its kind empty. A notice
// declaring ends_turn CLOSES A TURN — an assertion that a live process has
// stopped working — and that assertion must not be reachable by writing a string
// into a YAML file. Requiring a kind from the closed set obliges whoever adds a
// notice to add its kind in Go as well, where a reviewer has to look at what is
// being claimed and what it was measured against.
func (terminalNotices) Check(d *spec.Descriptor) error {
	for i, n := range d.TerminalNotices {
		if !hasContent(n.Needle) {
			return invalid(d.ID, "terminal_notices[%d]: needle must contain letters or digits", i)
		}
		if n.Kind == "" {
			return invalid(d.ID, "terminal_notices[%d]: kind is required", i)
		}
		if _, ok := spec.TerminalNoticeKinds[n.Kind]; !ok {
			return invalid(d.ID, "terminal_notices[%d]: unknown kind %q", i, n.Kind)
		}
	}
	return nil
}
