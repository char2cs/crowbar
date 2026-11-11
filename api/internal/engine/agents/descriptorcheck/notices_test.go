package descriptorcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// A notice matches only behind its anchor, so one declaring none never fires:
// a turn it should have ended stays open.
func TestValidate_ANoticeWithoutAnAnchorIsAnError(t *testing.T) {
	yaml := "terminal_notices:\n  - kind: usage_limit\n    needle: \"limit reached\"\n    ends_turn: true\n"
	f := findingFor(t, descriptorcheck.Validate(minimal(yaml)), "terminal_notices.anchor_missing")

	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
	assert.Equal(t, "terminal_notices[0]", f.Path)
	assert.Positive(t, f.Line)
}
