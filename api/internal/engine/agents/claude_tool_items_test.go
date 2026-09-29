package agents_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Claude reports its own tool names through generic hook fields. The descriptor,
// not the runtime or UI, translates those names into Crowbar's semantic kinds so
// both providers use the same rich activity rows.
func TestRegression_ClaudeToolCallsCarrySemanticKinds(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind string
	}{
		{name: "Read", kind: "read"},
		{name: "Write", kind: "edit"},
		{name: "Edit", kind: "edit"},
		{name: "NotebookEdit", kind: "edit"},
		{name: "Bash", kind: "execute"},
		{name: "Grep", kind: "search"},
		{name: "Glob", kind: "search"},
		{name: "WebFetch", kind: "fetch"},
		{name: "WebSearch", kind: "search"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := []byte(fmt.Sprintf(`{
				"session_id":"s1",
				"tool_use_id":"tool-1",
				"tool_name":%q,
				"tool_input":{"file_path":"/ws/note.txt"}
			}`, tc.name))

			ev, err := get(t, "claude").ParseHook(agents.HookToolPre, raw, agents.ChannelHooks)

			require.NoError(t, err)
			require.NotNil(t, ev.Tool)
			assert.Equal(t, tc.kind, ev.Tool.Kind)
			assert.Equal(t, tc.name, ev.Tool.Name)
			assert.Equal(t, "/ws/note.txt", ev.Tool.Target)
		})
	}
}
