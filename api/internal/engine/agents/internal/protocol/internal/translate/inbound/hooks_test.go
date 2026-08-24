package inbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func descriptor(events map[string]map[string]string, require_ ...string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{}}
	d.Runtime.Hooks.Format = "json"
	d.Runtime.Hooks.RequirePayloadFields = require_
	for canonical, fields := range events {
		d.Events[canonical] = spec.EventSpec{In: canonical, Map: fields}
	}
	return d
}

func TestParse_MapsTheConversationFields(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookUserPrompt: {"message": "prompt", "session_id": "session_id"},
	})

	ev, err := inbound.Parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi","session_id":"s1"}`))

	require.NoError(t, err)
	assert.Equal(t, spec.HookUserPrompt, ev.Kind)
	assert.Equal(t, "hi", ev.Message)
	assert.Equal(t, "s1", ev.SessionID)
}

func TestParse_UnmappedFieldsStayZero(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := inbound.Parse(d, spec.HookTurnStop, []byte(`{"last":"done","effort":{"level":"high"}}`))

	require.NoError(t, err)
	assert.Equal(t, "done", ev.Message)
	assert.Empty(t, ev.Effort, "a field the descriptor does not map must not be guessed at")
}

func TestParse_AnEmptyPayloadIsNotAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := inbound.Parse(d, spec.HookTurnStop, nil)

	require.NoError(t, err)
	assert.Empty(t, ev.Message)
}

func TestParse_MalformedPayloadIsAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	_, err := inbound.Parse(d, spec.HookTurnStop, []byte(`{not json`))

	assert.Error(t, err)
}

func TestParse_UnsupportedFormatIsAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})
	d.Runtime.Hooks.Format = "toml"

	_, err := inbound.Parse(d, spec.HookTurnStop, []byte(`{}`))

	assert.ErrorIs(t, err, inbound.ErrUnsupportedFormat)
}

func TestParse_AnUndeclaredEventIsReportedAsUndeclared(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	_, err := inbound.Parse(d, spec.HookNotification, []byte(`{}`))

	assert.ErrorIs(t, err, inbound.ErrUndeclaredEvent)
}

func TestParse_DropsAPayloadThatIsNotThisCLIsOwnConversation(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)

	_, err := inbound.Parse(d, spec.HookUserPrompt,
		[]byte(`{"prompt":"consolidate memories","transcript_path":null}`))

	require.ErrorIs(t, err, inbound.ErrForeignConversation)
	var foreign *inbound.ForeignConversationError
	require.ErrorAs(t, err, &foreign)
	assert.Equal(t, "transcript_path", foreign.Field,
		"the drop must say which declared field gave it away")
}

func TestParse_TreatsAnExplicitNullAsAbsentForTheOwnershipGuard(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)

	_, nullErr := inbound.Parse(d, spec.HookUserPrompt, []byte(`{"transcript_path":null}`))
	_, realErr := inbound.Parse(d, spec.HookUserPrompt,
		[]byte(`{"prompt":"hi","transcript_path":"/rollouts/x.jsonl"}`))

	assert.ErrorIs(t, nullErr, inbound.ErrForeignConversation)
	assert.NoError(t, realErr)
}

func TestParse_ADescriptorDeclaringNoGuardIsUnaffected(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}})

	_, err := inbound.Parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi","transcript_path":null}`))

	assert.NoError(t, err)
}

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

			ev, err := inbound.Parse(d, spec.HookTurnStop, []byte(tc.payload))

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

	pre, err := inbound.Parse(d, spec.HookToolPre,
		[]byte(`{"tool_use_id":"t1","tool_name":"Edit","tool_input":{"file_path":"a.go"}}`))
	require.NoError(t, err)
	require.NotNil(t, pre.Tool)
	assert.Equal(t, "t1", pre.Tool.ID)
	assert.Equal(t, "Edit", pre.Tool.Name)
	assert.Equal(t, "a.go", pre.Tool.Target)
	assert.JSONEq(t, `{"file_path":"a.go"}`, string(pre.Tool.Input))

	post, err := inbound.Parse(d, spec.HookToolPost,
		[]byte(`{"tool_use_id":"t1","tool_name":"Edit","tool_response":"ok","duration_ms":42}`))
	require.NoError(t, err)
	require.NotNil(t, post.Tool)
	assert.Equal(t, "ok", string(post.Tool.Result))
	assert.Equal(t, 42, post.Tool.DurationMS)
}

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
			ev, err := inbound.Parse(d, spec.HookToolPre, []byte(tc.payload))
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

	sub, err := inbound.Parse(d, spec.HookSubagentPre, []byte(`{"agent_id":"a1","agent_type":"explore"}`))
	require.NoError(t, err)
	require.NotNil(t, sub.Subagent)
	assert.Equal(t, "a1", sub.Subagent.ID)
	assert.Equal(t, "explore", sub.Subagent.AgentType)

	note, err := inbound.Parse(d, spec.HookNotification, []byte(`{"message":"waiting on you"}`))
	require.NoError(t, err)
	require.NotNil(t, note.Interrupt)
	assert.Equal(t, models.InterruptNotification, note.Interrupt.Kind)
	assert.Equal(t, "waiting on you", note.Interrupt.Detail)
	assert.False(t, note.Interrupt.Resolved)

	perm, err := inbound.Parse(d, spec.HookPermission, []byte(`{"tool_name":"Bash"}`))
	require.NoError(t, err)
	require.NotNil(t, perm.Interrupt)
	assert.Equal(t, models.InterruptPermission, perm.Interrupt.Kind)

	pre, err := inbound.Parse(d, spec.HookCompactPre, []byte(`{"trigger":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, pre.Interrupt)
	assert.Equal(t, models.InterruptCompaction, pre.Interrupt.Kind)
	assert.False(t, pre.Interrupt.Resolved)

	post, err := inbound.Parse(d, spec.HookCompactPost, []byte(`{"trigger":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, post.Interrupt)
	assert.True(t, post.Interrupt.Resolved, "a completed compaction is a resolved interruption")
}

func TestParse_CarriesTheRawPayload(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := inbound.Parse(d, spec.HookTurnStop, []byte(`{"last":"x","extra":1}`))

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

	assert.Equal(t, []string{"session_start", "tool_pre", "turn_stop"}, inbound.Declared(d))
	assert.Nil(t, inbound.Declared(nil))
}
