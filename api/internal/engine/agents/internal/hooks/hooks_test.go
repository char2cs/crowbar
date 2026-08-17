package hooks_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/hooks"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func descriptor(events map[string]map[string]string, require_ ...string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Hooks.Format = "json"
	d.Hooks.Events = events
	d.Hooks.RequirePayloadFields = require_
	return d
}

func TestParse_MapsTheConversationFields(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookUserPrompt: {"message": "prompt", "session_id": "session_id"},
	})

	ev, err := hooks.Parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi","session_id":"s1"}`))

	require.NoError(t, err)
	assert.Equal(t, spec.HookUserPrompt, ev.Kind)
	assert.Equal(t, "hi", ev.Message)
	assert.Equal(t, "s1", ev.SessionID)
}

func TestParse_UnmappedFieldsStayZero(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := hooks.Parse(d, spec.HookTurnStop, []byte(`{"last":"done","effort":{"level":"high"}}`))

	require.NoError(t, err)
	assert.Equal(t, "done", ev.Message)
	assert.Empty(t, ev.Effort, "a field the descriptor does not map must not be guessed at")
}

func TestParse_AnEmptyPayloadIsNotAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := hooks.Parse(d, spec.HookTurnStop, nil)

	require.NoError(t, err)
	assert.Empty(t, ev.Message)
}

func TestParse_MalformedPayloadIsAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	_, err := hooks.Parse(d, spec.HookTurnStop, []byte(`{not json`))

	assert.Error(t, err)
}

func TestParse_UnsupportedFormatIsAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})
	d.Hooks.Format = "toml"

	_, err := hooks.Parse(d, spec.HookTurnStop, []byte(`{}`))

	assert.ErrorIs(t, err, hooks.ErrUnsupportedFormat)
}

// A provider without a concept simply never reports it. That is an ordinary
// outcome the caller drops, not a failure.
func TestParse_AnUndeclaredEventIsReportedAsUndeclared(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	_, err := hooks.Parse(d, spec.HookNotification, []byte(`{}`))

	assert.ErrorIs(t, err, hooks.ErrUndeclaredEvent)
}

// A CLI's hooks are not only its user's. A provider's own internal work fires the
// same injected commands with a session id nobody has seen; what separates it is
// that it is not a conversation at all.
func TestParse_DropsAPayloadThatIsNotThisCLIsOwnConversation(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)

	_, err := hooks.Parse(d, spec.HookUserPrompt,
		[]byte(`{"prompt":"consolidate memories","transcript_path":null}`))

	require.ErrorIs(t, err, hooks.ErrForeignConversation)
	var foreign *hooks.ForeignConversationError
	require.ErrorAs(t, err, &foreign)
	assert.Equal(t, "transcript_path", foreign.Field,
		"the drop must say which declared field gave it away")
}

// An explicit JSON null decodes to a map entry that IS there, so a presence-only
// check would let the payload through and silently undo the guard.
func TestParse_TreatsAnExplicitNullAsAbsentForTheOwnershipGuard(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)

	_, nullErr := hooks.Parse(d, spec.HookUserPrompt, []byte(`{"transcript_path":null}`))
	_, realErr := hooks.Parse(d, spec.HookUserPrompt,
		[]byte(`{"prompt":"hi","transcript_path":"/rollouts/x.jsonl"}`))

	assert.ErrorIs(t, nullErr, hooks.ErrForeignConversation)
	assert.NoError(t, realErr)
}

func TestParse_ADescriptorDeclaringNoGuardIsUnaffected(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}})

	_, err := hooks.Parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi","transcript_path":null}`))

	assert.NoError(t, err)
}

// AsyncWork is a LEVEL the provider re-states, read as the length of a declared
// array. A provider that names nothing reports zero forever.
func TestParse_AsyncWorkIsTheLengthOfTheDeclaredArray(t *testing.T) {
	testCases := []struct {
		name    string
		mapping map[string]string
		payload string
		want    int
	}{
		{
			"three outstanding",
			map[string]string{"async_work": "background_tasks"},
			`{"background_tasks":[1,2,3]}`, 3,
		},
		{
			"converged to empty",
			map[string]string{"async_work": "background_tasks"},
			`{"background_tasks":[]}`, 0,
		},
		{
			"field absent on an older CLI",
			map[string]string{"async_work": "background_tasks"},
			`{}`, 0,
		},
		{"provider maps nothing", map[string]string{}, `{"background_tasks":[1,2]}`, 0},
		{
			"not an array",
			map[string]string{"async_work": "background_tasks"},
			`{"background_tasks":"running"}`, 0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mapping := map[string]string{"message": "last"}
			for k, v := range tc.mapping {
				mapping[k] = v
			}
			d := descriptor(map[string]map[string]string{spec.HookTurnStop: mapping})

			ev, err := hooks.Parse(d, spec.HookTurnStop, []byte(tc.payload))

			require.NoError(t, err)
			assert.Equal(t, tc.want, ev.AsyncWork)
		})
	}
}

func TestParse_BuildsAToolEventForBothToolPhases(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookToolPre: {
			"tool_id": "tool_use_id", "tool_name": "tool_name",
			"tool_target": "tool_input.file_path", "tool_input": "tool_input",
		},
		spec.HookToolPost: {
			"tool_id": "tool_use_id", "tool_name": "tool_name",
			"tool_result": "tool_response", "duration_ms": "duration_ms",
		},
	})

	pre, err := hooks.Parse(d, spec.HookToolPre,
		[]byte(`{"tool_use_id":"t1","tool_name":"Edit","tool_input":{"file_path":"a.go"}}`))
	require.NoError(t, err)
	require.NotNil(t, pre.Tool)
	assert.Equal(t, "t1", pre.Tool.ID)
	assert.Equal(t, "Edit", pre.Tool.Name)
	assert.Equal(t, "a.go", pre.Tool.Target)
	assert.JSONEq(t, `{"file_path":"a.go"}`, string(pre.Tool.Input))

	post, err := hooks.Parse(d, spec.HookToolPost,
		[]byte(`{"tool_use_id":"t1","tool_name":"Edit","tool_response":"ok","duration_ms":42}`))
	require.NoError(t, err)
	require.NotNil(t, post.Tool)
	assert.Equal(t, "ok", string(post.Tool.Result))
	assert.Equal(t, 42, post.Tool.DurationMS)
}

// One Crowbar concept lives at different payload paths depending on which tool
// fired: a file edit puts it in file_path, a shell call in command. Enumerating a
// provider's tool catalogue in the descriptor would go stale on every release.
func TestParse_ToolTargetTakesTheFirstMappedPathThatHasAValue(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookToolPre: {"tool_target": "tool_input.file_path,tool_input.command,tool_input.url"},
	})
	testCases := []struct {
		name    string
		payload string
		want    string
	}{
		{"file edit", `{"tool_input":{"file_path":"a.go"}}`, "a.go"},
		{"shell call", `{"tool_input":{"command":"go test ./..."}}`, "go test ./..."},
		{"fetch", `{"tool_input":{"url":"https://x"}}`, "https://x"},
		{"first wins", `{"tool_input":{"file_path":"a.go","command":"ls"}}`, "a.go"},
		{"none present", `{"tool_input":{}}`, ""},
		{"empty string is skipped", `{"tool_input":{"file_path":"","command":"ls"}}`, "ls"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := hooks.Parse(d, spec.HookToolPre, []byte(tc.payload))
			require.NoError(t, err)
			require.NotNil(t, ev.Tool)
			assert.Equal(t, tc.want, ev.Tool.Target)
		})
	}
}

func TestParse_BuildsSubagentAndInterruptEvents(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookSubagentPre:  {"subagent_id": "agent_id", "agent_type": "agent_type"},
		spec.HookSubagentPost: {"subagent_id": "agent_id"},
		spec.HookNotification: {"message": "message"},
		spec.HookPermission:   {"message": "tool_name"},
		spec.HookCompactPre:   {"trigger": "trigger"},
		spec.HookCompactPost:  {"trigger": "trigger"},
	})

	sub, err := hooks.Parse(d, spec.HookSubagentPre, []byte(`{"agent_id":"a1","agent_type":"explore"}`))
	require.NoError(t, err)
	require.NotNil(t, sub.Subagent)
	assert.Equal(t, "a1", sub.Subagent.ID)
	assert.Equal(t, "explore", sub.Subagent.AgentType)

	note, err := hooks.Parse(d, spec.HookNotification, []byte(`{"message":"waiting on you"}`))
	require.NoError(t, err)
	require.NotNil(t, note.Interrupt)
	assert.Equal(t, models.InterruptNotification, note.Interrupt.Kind)
	assert.Equal(t, "waiting on you", note.Interrupt.Detail)
	assert.False(t, note.Interrupt.Resolved)

	perm, err := hooks.Parse(d, spec.HookPermission, []byte(`{"tool_name":"Bash"}`))
	require.NoError(t, err)
	require.NotNil(t, perm.Interrupt)
	assert.Equal(t, models.InterruptPermission, perm.Interrupt.Kind)

	pre, err := hooks.Parse(d, spec.HookCompactPre, []byte(`{"trigger":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, pre.Interrupt)
	assert.Equal(t, models.InterruptCompaction, pre.Interrupt.Kind)
	assert.False(t, pre.Interrupt.Resolved)

	post, err := hooks.Parse(d, spec.HookCompactPost, []byte(`{"trigger":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, post.Interrupt)
	assert.True(t, post.Interrupt.Resolved, "a completed compaction is a resolved interruption")
}

func TestParse_CarriesTheRawPayload(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := hooks.Parse(d, spec.HookTurnStop, []byte(`{"last":"x","extra":1}`))

	require.NoError(t, err)
	assert.Equal(t, "x", ev.Raw["last"])
	assert.InDelta(t, 1.0, ev.Raw["extra"], 0.0001)
}

func TestDeclared_ListsTheMappedKindsSorted(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookTurnStop:     {},
		spec.HookSessionStart: {},
		spec.HookToolPre:      {},
	})

	assert.Equal(t, []string{"session_start", "tool_pre", "turn_stop"}, hooks.Declared(d))
	assert.Nil(t, hooks.Declared(nil))
}
