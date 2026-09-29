package turn

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestToolStatus_PreservesDeclinedAsATerminalState(t *testing.T) {
	event := engineagents.CanonicalEvent{
		Kind: engineagents.HookToolFail,
		Tool: &engineagents.ToolEvent{Status: domain.ToolStatusDeclined},
	}
	assert.Equal(t, domain.ToolStatusDeclined, toolStatus(event))
}

func TestToolStatus_OtherFailuresRemainErrors(t *testing.T) {
	event := engineagents.CanonicalEvent{
		Kind: engineagents.HookToolFail,
		Tool: &engineagents.ToolEvent{Status: "failed"},
	}
	assert.Equal(t, domain.ToolStatusError, toolStatus(event))
}
