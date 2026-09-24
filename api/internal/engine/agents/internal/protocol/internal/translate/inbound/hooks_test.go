package inbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// descriptor builds a probe descriptor from a single-path field map per event —
// every test in this file but one needs no more than one source path per
// canonical field. fieldMap does the []string lift.
func descriptor(events map[string]map[string]string, require_ ...string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{}}
	d.Runtime.Hooks.Format = "json"
	d.Runtime.Hooks.RequirePayloadFields = require_
	for canonical, fields := range events {
		d.Events[canonical] = spec.EventSpec{In: spec.WireRef{canonical}, Map: fieldMap(fields)}
	}
	return d
}

func fieldMap(fields map[string]string) spec.FieldMap {
	out := make(spec.FieldMap, len(fields))
	for k, v := range fields {
		out[k] = []string{v}
	}
	return out
}

// descriptorWithFields is descriptor()'s counterpart for a test that needs a
// REAL multi-path (first_present:-shaped) field, which a plain
// map[string]string cannot express.
func descriptorWithFields(canonical string, fields spec.FieldMap, require_ ...string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		canonical: {In: spec.WireRef{canonical}, Map: fields},
	}}
	d.Runtime.Hooks.Format = "json"
	d.Runtime.Hooks.RequirePayloadFields = require_
	return d
}

// parse is inbound.Parse pinned to the hooks channel. Every descriptor this
// file builds is LEGACY form (no channel blocks — see descriptor() above),
// so which channel is named does not change the result; the part of Parse
// that actually depends on channel is covered on its own below by
// TestParse_ChannelSelectsItsOwnBlock and its neighbours.
func parse(d *spec.Descriptor, canonical string, raw []byte) (models.CanonicalEvent, error) {
	return inbound.Parse(d, canonical, raw, spec.ChannelHooks)
}

func TestParse_MapsTheConversationFields(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookUserPrompt: {"message": "prompt", "session_id": "session_id"},
	})

	ev, err := parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi","session_id":"s1"}`))

	require.NoError(t, err)
	assert.Equal(t, spec.HookUserPrompt, ev.Kind)
	assert.Equal(t, "hi", ev.Message)
	assert.Equal(t, "s1", ev.SessionID)
}

// --- required: (design spec 2.3) -------------------------------------------

func TestParse_RequiredFieldMissingIsAHardError(t *testing.T) {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookUserPrompt: {
			Required: []string{"session_id", "message"},
			In:       spec.WireRef{spec.HookUserPrompt},
			Map:      spec.FieldMap{"session_id": {"session_id"}, "message": {"prompt"}},
		},
	}}
	d.Runtime.Hooks.Format = "json"

	_, err := parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi"}`))

	require.Error(t, err)
	var missing *inbound.RequiredFieldError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "probe", missing.Descriptor)
	assert.Equal(t, spec.HookUserPrompt, missing.Event)
	assert.Equal(t, "session_id", missing.Field, "must name the SPECIFIC missing field")
}

// A required field present but resolving to the empty string is exactly as
// missing as an absent key — an empty session id names no conversation.
func TestParse_RequiredFieldPresentButEmptyIsStillAHardError(t *testing.T) {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookUserPrompt: {
			Required: []string{"session_id"},
			In:       spec.WireRef{spec.HookUserPrompt},
			Map:      spec.FieldMap{"session_id": {"session_id"}},
		},
	}}
	d.Runtime.Hooks.Format = "json"

	_, err := parse(d, spec.HookUserPrompt, []byte(`{"session_id":""}`))

	require.Error(t, err)
	var missing *inbound.RequiredFieldError
	require.ErrorAs(t, err, &missing)
	assert.Equal(t, "session_id", missing.Field)
}

func TestParse_EveryRequiredFieldPresentSucceeds(t *testing.T) {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookUserPrompt: {
			Required: []string{"session_id", "message"},
			In:       spec.WireRef{spec.HookUserPrompt},
			Map:      spec.FieldMap{"session_id": {"session_id"}, "message": {"prompt"}},
		},
	}}
	d.Runtime.Hooks.Format = "json"

	ev, err := parse(d, spec.HookUserPrompt, []byte(`{"session_id":"s1","prompt":"hi"}`))

	require.NoError(t, err)
	assert.Equal(t, "s1", ev.SessionID)
	assert.Equal(t, "hi", ev.Message)
}

// An event with no required: list at all is untouched — this is the
// pre-existing, unmigrated shape most events still have.
func TestParse_NoRequiredListMeansNoCheck(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}})

	_, err := parse(d, spec.HookUserPrompt, []byte(`{}`))

	assert.NoError(t, err)
}

func TestParse_UnmappedFieldsStayZero(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := parse(d, spec.HookTurnStop, []byte(`{"last":"done","effort":{"level":"high"}}`))

	require.NoError(t, err)
	assert.Equal(t, "done", ev.Message)
	assert.Empty(t, ev.Effort, "a field the descriptor does not map must not be guessed at")
}

func TestParse_AnEmptyPayloadIsNotAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := parse(d, spec.HookTurnStop, nil)

	require.NoError(t, err)
	assert.Empty(t, ev.Message)
}

func TestParse_MalformedPayloadIsAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	_, err := parse(d, spec.HookTurnStop, []byte(`{not json`))

	assert.Error(t, err)
}

func TestParse_UnsupportedFormatIsAnError(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})
	d.Runtime.Hooks.Format = "toml"

	_, err := parse(d, spec.HookTurnStop, []byte(`{}`))

	assert.ErrorIs(t, err, inbound.ErrUnsupportedFormat)
}

func TestParse_AnUndeclaredEventIsReportedAsUndeclared(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	_, err := parse(d, spec.HookNotification, []byte(`{}`))

	assert.ErrorIs(t, err, inbound.ErrUndeclaredEvent)
}

func TestParse_DropsAPayloadThatIsNotThisCLIsOwnConversation(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)

	_, err := parse(d, spec.HookUserPrompt,
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

	_, nullErr := parse(d, spec.HookUserPrompt, []byte(`{"transcript_path":null}`))
	_, realErr := parse(d, spec.HookUserPrompt,
		[]byte(`{"prompt":"hi","transcript_path":"/rollouts/x.jsonl"}`))

	assert.ErrorIs(t, nullErr, inbound.ErrForeignConversation)
	assert.NoError(t, realErr)
}

func TestParse_ADescriptorDeclaringNoGuardIsUnaffected(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}})

	_, err := parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi","transcript_path":null}`))

	assert.NoError(t, err)
}

// TestRegression_ADualShapeEventStillCatchesAForeignHooksPayload is the bug
// reported live: codex's session_start/user_prompt/turn_stop declare no
// per-event transport override, so on codex's own mixed-transport descriptor
// they inherit the runtime default of "api" — but codex's spawn config still
// ALSO fires them hooks-shaped for its internal memory-consolidation session
// (transcript_path: null, byte-identical to a real /new otherwise). The guard
// used to be skipped outright for any event whose DECLARED transport is api,
// which let this exact payload through and re-opened the chat-theft bug
// require_payload_fields exists to close. Confirmed live against the real
// codex.yaml before this fix: accepted, no rejection.
func TestRegression_ADualShapeEventStillCatchesAForeignHooksPayload(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)
	d.Runtime.Transport = "api"
	// No per-event Transport override — exactly codex.yaml's own shape for
	// session_start/user_prompt/turn_stop, inheriting the runtime default.

	_, err := parse(d, spec.HookUserPrompt,
		[]byte(`{"prompt":"MEMORY-WRITING-AGENT-PHASE-2-CONSOLIDATION","transcript_path":null}`))

	require.ErrorIs(t, err, inbound.ErrForeignConversation,
		"a hooks-shaped delivery of a dual-shape event must still be checked, even though the "+
			"EVENT's declared transport is api")
}

// TestParse_ADualShapeEventStillAcceptsAGenuineAPIPayload guards the failure
// mode the old transport-wide skip existed to prevent in the first place: an
// api-transport payload structurally never carries transcript_path at all
// (absent, not empty), and must not be rejected for lacking it.
func TestParse_ADualShapeEventStillAcceptsAGenuineAPIPayload(t *testing.T) {
	d := descriptor(
		map[string]map[string]string{spec.HookUserPrompt: {"message": "prompt"}},
		"transcript_path",
	)
	d.Runtime.Transport = "api"

	_, err := parse(d, spec.HookUserPrompt, []byte(`{"prompt":"hi"}`))

	assert.NoError(t, err, "an api-transport payload never carries transcript_path; its absence is not foreign")
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

			ev, err := parse(d, spec.HookTurnStop, []byte(tc.payload))

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

	pre, err := parse(d, spec.HookToolPre,
		[]byte(`{"tool_use_id":"t1","tool_name":"Edit","tool_input":{"file_path":"a.go"}}`))
	require.NoError(t, err)
	require.NotNil(t, pre.Tool)
	assert.Equal(t, "t1", pre.Tool.ID)
	assert.Equal(t, "Edit", pre.Tool.Name)
	assert.Equal(t, "a.go", pre.Tool.Target)
	assert.JSONEq(t, `{"file_path":"a.go"}`, string(pre.Tool.Input))

	post, err := parse(d, spec.HookToolPost,
		[]byte(`{"tool_use_id":"t1","tool_name":"Edit","tool_response":"ok","duration_ms":42}`))
	require.NoError(t, err)
	require.NotNil(t, post.Tool)
	assert.Equal(t, "ok", string(post.Tool.Result))
	assert.Equal(t, 42, post.Tool.DurationMS)
}

// A tool call may name a whole SECOND conversation, not just its own
// request/result — codex's collabAgentToolCall reports the thread id of the
// agent it spawned or is addressing. NestedSessionID is generic: the
// descriptor supplies the path, Go never learns why the field is there.
func TestParse_ToolPostCarriesANestedSessionIDWhenTheDescriptorMapsOne(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookToolPost: {
			"tool_id": "tool_use_id", "nested_session_id": "receiver_thread",
		},
	})

	ev, err := parse(d, spec.HookToolPost,
		[]byte(`{"tool_use_id":"t1","receiver_thread":"child-1"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "child-1", ev.Tool.NestedSessionID)
}

func TestParse_ToolPostWithNoNestedSessionMappingLeavesItEmpty(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookToolPost: {"tool_id": "tool_use_id"},
	})

	ev, err := parse(d, spec.HookToolPost, []byte(`{"tool_use_id":"t1"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Empty(t, ev.Tool.NestedSessionID)
}

func TestParse_ToolTargetTakesTheFirstMappedPathThatHasAValue(t *testing.T) {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookToolPre: {
			In: spec.WireRef{spec.HookToolPre},
			Map: spec.FieldMap{
				"tool_target": {"tool_input.file_path", "tool_input.command", "tool_input.url"},
			},
		},
	}}
	d.Runtime.Hooks.Format = "json"
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
			ev, err := parse(d, spec.HookToolPre, []byte(tc.payload))
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

	sub, err := parse(d, spec.HookSubagentPre, []byte(`{"agent_id":"a1","agent_type":"explore"}`))
	require.NoError(t, err)
	require.NotNil(t, sub.Subagent)
	assert.Equal(t, "a1", sub.Subagent.ID)
	assert.Equal(t, "explore", sub.Subagent.AgentType)

	note, err := parse(d, spec.HookNotification, []byte(`{"message":"waiting on you"}`))
	require.NoError(t, err)
	require.NotNil(t, note.Interrupt)
	assert.Equal(t, models.InterruptNotification, note.Interrupt.Kind)
	assert.Equal(t, "waiting on you", note.Interrupt.Detail)
	assert.False(t, note.Interrupt.Resolved)

	perm, err := parse(d, spec.HookPermission, []byte(`{"tool_name":"Bash"}`))
	require.NoError(t, err)
	require.NotNil(t, perm.Interrupt)
	assert.Equal(t, models.InterruptPermission, perm.Interrupt.Kind)

	pre, err := parse(d, spec.HookCompactPre, []byte(`{"trigger":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, pre.Interrupt)
	assert.Equal(t, models.InterruptCompaction, pre.Interrupt.Kind)
	assert.False(t, pre.Interrupt.Resolved)

	post, err := parse(d, spec.HookCompactPost, []byte(`{"trigger":"auto"}`))
	require.NoError(t, err)
	require.NotNil(t, post.Interrupt)
	assert.True(t, post.Interrupt.Resolved, "a completed compaction is a resolved interruption")
}

func TestParse_CarriesTheRawPayload(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookTurnStop: {"message": "last"}})

	ev, err := parse(d, spec.HookTurnStop, []byte(`{"last":"x","extra":1}`))

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

// --- channel selection: the chat-theft class, made explicit -----------------

// channelSplitDescriptor is the design spec's own tool_pre example
// (docs/plans/2026-09-22-descriptor-channel-split.md, 2.1): ONE canonical
// event, two channel blocks with DIFFERENT wire names and DIFFERENT field
// maps for the same conversational fact.
func channelSplitDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookToolPre: {
			Required: []string{"session_id", "tool_id", "tool_name"},
			API: &spec.ChannelBlock{
				In:   spec.WireRef{"item/started"},
				When: spec.WhenMap{"item.type": {"commandExecution", "fileChange"}},
				Map: spec.FieldMap{
					"session_id": {"threadId"}, "tool_id": {"item.id"}, "tool_name": {"item.type"},
				},
			},
			Hooks: &spec.ChannelBlock{
				In: spec.WireRef{"PreToolUse"},
				Map: spec.FieldMap{
					"session_id": {"session_id"}, "tool_id": {"tool_use_id"}, "tool_name": {"tool_name"},
				},
			},
		},
	}}
	d.Runtime.Hooks.Format = "json"
	return d
}

// TestParse_ChannelSelectsItsOwnBlock is the chat-theft class made explicit:
// the SAME canonical event, the SAME descriptor, two payloads shaped for two
// DIFFERENT channels. This package's own doc comment on Parse (and hooks.go's
// on ownsConversation) records the live bug this replaces — a canonical event
// resolved by its STATIC declared transport, instead of the channel THIS
// delivery actually arrived on, let a foreign hooks-shaped payload of an
// api-default event through unchecked. Parse must read each payload through
// its OWN channel's block.
func TestParse_ChannelSelectsItsOwnBlock(t *testing.T) {
	d := channelSplitDescriptor()

	apiPayload := []byte(`{"threadId":"api-session","item":{"type":"commandExecution","id":"api-tool"}}`)
	ev, err := inbound.Parse(d, spec.HookToolPre, apiPayload, spec.ChannelAPI)
	require.NoError(t, err)
	assert.Equal(t, "api-session", ev.SessionID)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "api-tool", ev.Tool.ID)
	assert.Equal(t, "commandExecution", ev.Tool.Name)

	hooksPayload := []byte(`{"session_id":"hooks-session","tool_use_id":"hooks-tool","tool_name":"Bash"}`)
	ev, err = inbound.Parse(d, spec.HookToolPre, hooksPayload, spec.ChannelHooks)
	require.NoError(t, err)
	assert.Equal(t, "hooks-session", ev.SessionID)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "hooks-tool", ev.Tool.ID)
	assert.Equal(t, "Bash", ev.Tool.Name)
}

// TestRegression_ChannelSelectionDoesNotReadTheOtherShapesFields is the
// isolation proof: parsing a payload through the WRONG channel's block must
// resolve NOTHING — the api-shaped payload's threadId/item.id must not
// accidentally satisfy the hooks block's session_id/tool_use_id paths (and
// vice versa). If channel selection degenerated back into reading both
// shapes at once (the old || cross-shape fallback this design replaces),
// this would start passing something.
//
// tool_pre declares session_id required:, so the isolation now surfaces as a
// RequiredFieldError rather than a successful event with empty fields — a
// STRONGER proof than before: the wrong-channel delivery is not just hollow,
// it is REJECTED, exactly what required: (design spec 2.3) exists for.
//
// The api-channel direction is rejected EARLIER than that, and by something
// stricter: the api block's own when: (item.type) cannot resolve in a
// hooks-shaped payload at all, so the discriminator refuses the delivery
// before required: is ever consulted. Both halves still prove the one thing
// this test exists for — a payload read through the wrong channel's block
// resolves NOTHING and is refused, never half-parsed.
func TestRegression_ChannelSelectionDoesNotReadTheOtherShapesFields(t *testing.T) {
	d := channelSplitDescriptor()

	apiPayload := []byte(`{"threadId":"api-session","item":{"type":"commandExecution","id":"api-tool"}}`)
	_, err := inbound.Parse(d, spec.HookToolPre, apiPayload, spec.ChannelHooks)
	var missing *inbound.RequiredFieldError
	require.ErrorAs(t, err, &missing,
		"the hooks block's session_id: path does not exist in an api-shaped payload")
	assert.Equal(t, "session_id", missing.Field)

	hooksPayload := []byte(`{"session_id":"hooks-session","tool_use_id":"hooks-tool","tool_name":"Bash"}`)
	_, err = inbound.Parse(d, spec.HookToolPre, hooksPayload, spec.ChannelAPI)
	var mismatch *inbound.VariantMismatchError
	require.ErrorAs(t, err, &mismatch,
		"the api block's item.type: discriminator does not resolve in a hooks-shaped payload")
	assert.Equal(t, string(spec.ChannelAPI), mismatch.Channel)
	assert.NotErrorAs(t, err, &missing,
		"a payload that is not even this block's variant must not be reported as a "+
			"missing required field")
}

// TestParse_AChannelTheEventDoesNotDeclareIsRefusedNotHalfParsed is design
// spec 2.1's own consequence: "a channel a provider does not use is simply
// absent; its payloads are refused with a named error rather than silently
// half-parsed."
func TestParse_AChannelTheEventDoesNotDeclareIsRefusedNotHalfParsed(t *testing.T) {
	d := channelSplitDescriptor()

	_, err := inbound.Parse(d, spec.HookToolPre, []byte(`{}`), spec.Channel("oneshot"))

	assert.ErrorIs(t, err, inbound.ErrUndeclaredEvent)
}

// permissionChannelSplitDescriptor mirrors codex.yaml's own migrated
// permission shape (docs/plans/2026-09-22-descriptor-channel-split.md P2b):
// an ASK-direction event, channel-split, api: and hooks: naming DIFFERENT
// wire methods and DIFFERENT field paths for the same conversational fact.
func permissionChannelSplitDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookPermission: {
			Required: []string{"session_id", "tool_name"},
			API: &spec.ChannelBlock{
				Ask: spec.WireRef{"approval/request"},
				Map: spec.FieldMap{
					"session_id": {"threadId"}, "message": {"reason"}, "tool_name": {"tool"},
				},
			},
			Hooks: &spec.ChannelBlock{
				Ask: spec.WireRef{"PermissionRequest"},
				Map: spec.FieldMap{
					"session_id": {"session_id"}, "message": {"message"}, "tool_name": {"tool_name"},
				},
			},
			Reply: map[string]string{"allow": `{"decision":"accept"}`},
		},
	}}
	d.Runtime.Hooks.Format = "json"
	return d
}

// TestRegression_AskChannelSelectionDoesNotReadTheOtherShapesFields is
// TestRegression_ChannelSelectionDoesNotReadTheOtherShapesFields's
// ASK-direction counterpart (the P3 gap report this phase closes): permission
// resolving through the WRONG channel's block must resolve NOTHING — the
// api-shaped payload's threadId/reason/tool must not satisfy the hooks
// block's session_id/message/tool_name paths, and vice versa. This is the
// same chat-theft class TestRegression_ACodexPermissionIsAnsweredFromCrowbar
// AndReachesTheCLI (internal/app/usecases/chat/answers_test.go) exercises
// end to end; this pins the field-resolution level underneath it.
//
// permission declares session_id required:, so — same upgrade as
// TestRegression_ChannelSelectionDoesNotReadTheOtherShapesFields — the
// isolation now surfaces as a RequiredFieldError, not a hollow success.
func TestRegression_AskChannelSelectionDoesNotReadTheOtherShapesFields(t *testing.T) {
	d := permissionChannelSplitDescriptor()

	apiPayload := []byte(`{"threadId":"api-session","reason":"api reason","tool":"api-tool"}`)
	_, err := inbound.Parse(d, spec.HookPermission, apiPayload, spec.ChannelHooks)
	var missing *inbound.RequiredFieldError
	require.ErrorAs(t, err, &missing,
		"the hooks block's session_id: path does not exist in an api-shaped payload")
	assert.Equal(t, "session_id", missing.Field)

	hooksPayload := []byte(
		`{"session_id":"hooks-session","message":"hooks message","tool_name":"hooks-tool"}`)
	_, err = inbound.Parse(d, spec.HookPermission, hooksPayload, spec.ChannelAPI)
	require.ErrorAs(t, err, &missing,
		"the api block's threadId: path does not exist in a hooks-shaped payload")
	assert.Equal(t, "session_id", missing.Field)
}

// TestParse_AskChannelSelectsItsOwnBlock is
// TestParse_ChannelSelectsItsOwnBlock's ask-direction counterpart: the
// GENUINE delivery on each channel must still resolve correctly through its
// own block — the isolation proof above only means something once the happy
// path is proven too.
func TestParse_AskChannelSelectsItsOwnBlock(t *testing.T) {
	d := permissionChannelSplitDescriptor()

	apiPayload := []byte(`{"threadId":"api-session","reason":"api reason","tool":"api-tool"}`)
	ev, err := inbound.Parse(d, spec.HookPermission, apiPayload, spec.ChannelAPI)
	require.NoError(t, err)
	assert.Equal(t, "api-session", ev.SessionID)
	assert.Equal(t, "api reason", ev.Message)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, "api-tool", ev.Choice.ToolName)

	hooksPayload := []byte(
		`{"session_id":"hooks-session","message":"hooks message","tool_name":"hooks-tool"}`)
	ev, err = inbound.Parse(d, spec.HookPermission, hooksPayload, spec.ChannelHooks)
	require.NoError(t, err)
	assert.Equal(t, "hooks-session", ev.SessionID)
	assert.Equal(t, "hooks message", ev.Message)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, "hooks-tool", ev.Choice.ToolName)
}

// --- when: on the hooks channel --------------------------------------------
//
// `when:` used to be read by dispatch.Resolve alone, which the API transport is
// the only caller of: it turns a provider's own method name back into a
// canonical event, and a sum-type method needs the discriminator to pick which.
// The hooks channel never needed it, because a hook relay is invoked WITH the
// canonical name (settings.json bakes one command per wire hook), so there was
// nothing to resolve.
//
// That made one wire hook mean exactly one canonical event, forever — and claude
// needs one wire hook (Notification) to mean two: an idle report when it says it
// is waiting for the human, and nothing at all when the same hook carries a
// permission prompt. Two relay commands deliver it twice under two canonical
// names, and the `when:` below is the only thing that can tell the two deliveries
// apart. Without it the permission copy would arm the idle latch and
// AbandonMessage would kill a live turn five seconds later.

// idleVariantDescriptor is the two-events-one-wire-hook shape: `idle` is gated
// on a discriminator, `notification` takes every delivery of the same hook.
func idleVariantDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookIdle: {
			In:   spec.WireRef{"Notification"},
			When: spec.WhenMap{"message": {"waiting for input"}},
			Map:  spec.FieldMap{"session_id": {"session_id"}},
		},
		spec.HookNotification: {
			In:  spec.WireRef{"Notification"},
			Map: spec.FieldMap{"session_id": {"session_id"}, "message": {"message"}},
		},
	}}
	d.Runtime.Hooks.Format = "json"
	return d
}

func TestParse_HooksChannelAppliesTheWhenDiscriminator(t *testing.T) {
	d := idleVariantDescriptor()

	ev, err := parse(d, spec.HookIdle, []byte(`{"session_id":"s1","message":"waiting for input"}`))

	require.NoError(t, err)
	assert.Equal(t, spec.HookIdle, ev.Kind)
	assert.Equal(t, "s1", ev.SessionID)
}

func TestParse_HooksChannelRefusesADeliveryItsWhenDoesNotMatch(t *testing.T) {
	d := idleVariantDescriptor()

	_, err := parse(d, spec.HookIdle, []byte(`{"session_id":"s1","message":"needs permission"}`))

	require.Error(t, err)
	assert.ErrorIs(t, err, inbound.ErrVariantMismatch)
	var mismatch *inbound.VariantMismatchError
	require.ErrorAs(t, err, &mismatch)
	assert.Equal(t, "probe", mismatch.Descriptor)
	assert.Equal(t, spec.HookIdle, mismatch.Event)
	assert.Equal(t, string(spec.ChannelHooks), mismatch.Channel)
}

// The UNGATED sibling of the same wire hook still takes every delivery: a
// discriminator selects one variant IN, it never filters the others OUT.
func TestParse_AnUngatedEventTakesEveryDeliveryOfTheSameWireHook(t *testing.T) {
	d := idleVariantDescriptor()

	for _, message := range []string{"waiting for input", "needs permission"} {
		ev, err := parse(d, spec.HookNotification,
			[]byte(`{"session_id":"s1","message":"`+message+`"}`))

		require.NoError(t, err, message)
		assert.Equal(t, spec.HookNotification, ev.Kind)
		assert.Equal(t, message, ev.Message)
	}
}

// A payload missing the discriminator field entirely is a MISS, never a match —
// mapping.Match's own rule, asserted here because a variant selector that
// applied to payloads lacking the discriminator would be the unsafe direction.
func TestParse_ADeliveryWithNoDiscriminatorFieldIsRefused(t *testing.T) {
	d := idleVariantDescriptor()

	_, err := parse(d, spec.HookIdle, []byte(`{"session_id":"s1"}`))

	assert.ErrorIs(t, err, inbound.ErrVariantMismatch)
}

// The discriminator runs BEFORE required:, so the error a mismatched variant
// reports names the real reason rather than blaming a field the other variant
// happens not to carry.
func TestParse_TheDiscriminatorIsCheckedBeforeRequiredFields(t *testing.T) {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookIdle: {
			Required: []string{"session_id"},
			In:       spec.WireRef{"Notification"},
			When:     spec.WhenMap{"message": {"waiting for input"}},
			Map:      spec.FieldMap{"session_id": {"session_id"}},
		},
	}}
	d.Runtime.Hooks.Format = "json"

	_, err := parse(d, spec.HookIdle, []byte(`{"message":"needs permission"}`))

	assert.ErrorIs(t, err, inbound.ErrVariantMismatch)
	var missing *inbound.RequiredFieldError
	assert.NotErrorAs(t, err, &missing,
		"a different variant of the same hook is not a missing required field")
}

// A channel-scoped event reads its OWN block's when:, the same way it already
// reads its own map: — WhenFor(channel), never the other channel's.
func TestParse_AChannelScopedEventReadsItsOwnBlocksWhen(t *testing.T) {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookIdle: {
			Hooks: &spec.ChannelBlock{
				In:   spec.WireRef{"Notification"},
				When: spec.WhenMap{"message": {"hooks-idle"}},
				Map:  spec.FieldMap{"session_id": {"session_id"}},
			},
			API: &spec.ChannelBlock{
				In:   spec.WireRef{"status/changed"},
				When: spec.WhenMap{"status": {"idle"}},
				Map:  spec.FieldMap{"session_id": {"threadId"}},
			},
		},
	}}
	d.Runtime.Hooks.Format = "json"

	_, err := inbound.Parse(d, spec.HookIdle,
		[]byte(`{"session_id":"s1","status":"idle"}`), spec.ChannelHooks)
	assert.ErrorIs(t, err, inbound.ErrVariantMismatch,
		"the api block's own discriminator must not satisfy a hooks delivery")

	ev, err := inbound.Parse(d, spec.HookIdle,
		[]byte(`{"session_id":"s1","message":"hooks-idle"}`), spec.ChannelHooks)
	require.NoError(t, err)
	assert.Equal(t, "s1", ev.SessionID)
}
