package rules

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type terminalNotices struct{}

func (terminalNotices) Name() string { return "terminal_notices" }

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
