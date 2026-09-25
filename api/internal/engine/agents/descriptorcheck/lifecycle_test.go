package descriptorcheck_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// A descriptor that maps no turn close leaves every chat spinning forever.
func TestValidate_AMissingLifecycleEventIsAnError(t *testing.T) {
	raw := strings.Replace(string(complete("")),
		"  turn_stop: { required: [session_id], in: Stop, map: { session_id: session_id, message: last_assistant_message } }\n", "", 1)
	f := findingFor(t, descriptorcheck.Validate([]byte(raw)), "events.lifecycle_missing")
	assert.Contains(t, f.Message, "turn_stop")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
}

// A split event only arrives on the channels it has blocks for, so a surface
// whose channel it lacks never sees its turns close.
func TestValidate_ASurfaceThatCannotReceiveALifecycleEventIsAnError(t *testing.T) {
	raw := strings.Replace(string(complete("surfaces:\n  terminal: { channel: hooks, start_here: true }\n")),
		"  turn_stop: { required: [session_id], in: Stop, map: { session_id: session_id, message: last_assistant_message } }\n",
		"  turn_stop:\n    required: [session_id]\n    api: { in: turn/completed, map: { session_id: threadId, message: msg } }\n", 1)
	f := findingFor(t, descriptorcheck.Validate([]byte(raw)), "events.channel_missing")
	assert.Equal(t, "events.turn_stop", f.Path)
	assert.Contains(t, f.Hint, "events.turn_stop.hooks")
}

func TestValidate_NoPermissionEventIsAWarning(t *testing.T) {
	rep := descriptorcheck.Validate(complete(""))
	f := findingFor(t, rep, "events.permission_missing")
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
	assert.True(t, rep.OK())
}
