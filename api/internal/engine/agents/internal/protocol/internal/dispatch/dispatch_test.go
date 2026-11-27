package dispatch_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/dispatch"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/answer"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

const commandExecutionApproval = `{
  "threadId": "01a090df-569c-7393-a274-9d92400ce8eb",
  "turnId": "01a090df-57c4-7a73-ad72-03021a76744c",
  "itemId": "exec-b66701af-f5d8-4eaa-bc72-e73629df4aae",
  "startedAtMs": 1789136961566,
  "environmentId": "local",
  "reason": "Allow the exact curl command to access example.com outside the restricted sandbox?",
  "command": "/bin/zsh -lc 'curl -sS https://example.com -o /dev/null -w \"%{http_code}\"'",
  "cwd": "/private/tmp/cbx/probe",
  "commandActions": [{"type": "unknown", "command": "curl -sS https://example.com"}],
  "proposedExecpolicyAmendment": ["curl", "-sS", "https://example.com"],
  "availableDecisions": ["accept", {"acceptWithExecpolicyAmendment": {"execpolicy_amendment": ["curl"]}}, "cancel"]
}`

const fileChangeApproval = `{
  "threadId": "01a090e0-9b11-76c3-bdaf-452c82f7883b",
  "turnId": "01a090e0-9c3f-7db3-a72d-c593e8304716",
  "itemId": "exec-7d03abcf-7b47-4215-bc17-520bd278663a",
  "startedAtMs": 1789137039675,
  "reason": null,
  "grantRoot": null
}`

func loadCodexAPIDescriptor(t *testing.T) *spec.Descriptor {
	t.Helper()
	raw, err := os.ReadFile("../descriptor/descriptors-v3/codex.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV0(raw)
	require.NoError(t, err)
	return d
}

func loadParams(t *testing.T, fixture string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/fixtures/codex/" + fixture + ".json")
	require.NoError(t, err)
	var frame struct {
		Params map[string]any `json:"params"`
	}
	require.NoError(t, json.Unmarshal(raw, &frame))
	return frame.Params
}

func TestResolve_PlainEventNoSumType(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "turn/completed", loadParams(t, "turn_completed"))
	require.True(t, ok)
	assert.Equal(t, "turn_stop", canonical)
}

func TestResolve_SumTypeDisambiguatesByItemType(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	params := loadParams(t, "item_started")
	item, _ := params["item"].(map[string]any)
	require.Equal(t, "userMessage", item["type"], "fixture must carry the discriminator this test asserts on")

	canonical, ok := dispatch.Resolve(d, "item/started", params)
	require.True(t, ok)
	assert.Equal(t, "user_prompt", canonical)
}

func TestResolve_ACodexV2SpawnIsASubagentStart(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	for _, fixture := range []string{"item_started.subAgentActivity", "item_started.subAgentActivity.interacted"} {
		canonical, ok := dispatch.Resolve(d, "item/started", loadParams(t, fixture))

		require.True(t, ok, fixture)
		assert.Equal(t, "subagent_pre", canonical, fixture)
	}
}

func TestResolve_UnknownWireMethodIsNotOK(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "some/method/nobody/declared", map[string]any{})
	assert.False(t, ok)
}

func TestResolve_OutboundEventsAreNeverCandidates(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "turn/start", map[string]any{})
	assert.False(t, ok, "turn/start is what WE send, not something codex reports")
}

func TestResolve_AFlatHooksEventIsNeverAnAPICandidate(t *testing.T) {
	d := &spec.Descriptor{ID: "p", Runtime: spec.RuntimeSpec{Transport: "api"}, Events: spec.EventTable{
		"subagent_pre": {Transport: "hooks", In: spec.WireRef{"SubagentStart"}},
	}}
	_, ok := dispatch.Resolve(d, "SubagentStart", map[string]any{})
	assert.False(t, ok)
}

func TestResolve_AskEventsAreCandidatesToo(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "item/commandExecution/requestApproval", map[string]any{"tool": "shell"})
	require.True(t, ok)
	assert.Equal(t, "permission", canonical)
}

func TestResolve_WhenClauseThatDoesNotMatchFallsThrough(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "item/started", map[string]any{
		"item": map[string]any{"type": "reasoning"},
	})
	assert.False(t, ok)
}

func TestRegression_AFailedTurnRoutesToTurnFailedNotTurnStop(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	params := loadParams(t, "turn_completed.failed-schema")
	turn, _ := params["turn"].(map[string]any)
	require.Equal(t, "failed", turn["status"], "fixture must carry the discriminator")

	canonical, ok := dispatch.Resolve(d, "turn/completed", params)

	require.True(t, ok)
	assert.Equal(t, "turn_failed", canonical)
}

func TestResolve_ChannelScopedEventUsesItsOwnAPIBlock(t *testing.T) {
	d := &spec.Descriptor{Events: map[string]spec.EventSpec{
		"tool_pre": {
			API: &spec.ChannelBlock{
				In:   spec.WireRef{"item/started"},
				When: spec.WhenMap{"item.type": {"commandExecution", "fileChange"}},
				Map:  spec.FieldMap{"tool_id": mapping.Paths("item.id")},
			},
			Hooks: &spec.ChannelBlock{In: spec.WireRef{"PreToolUse"}, Map: spec.FieldMap{"tool_id": mapping.Paths("tool_use_id")}},
		},
	}}

	canonical, ok := dispatch.Resolve(d, "item/started", map[string]any{
		"item": map[string]any{"type": "commandExecution", "id": "i1"},
	})
	require.True(t, ok, "the api block's own in: must resolve the wire method")
	assert.Equal(t, "tool_pre", canonical)

	_, ok = dispatch.Resolve(d, "PreToolUse", map[string]any{})
	assert.False(t, ok, "a channel-scoped event's hooks: wire name must not resolve on the api transport")

	_, ok = dispatch.Resolve(d, "item/started", map[string]any{
		"item": map[string]any{"type": "reasoning"},
	})
	assert.False(t, ok, "the api block's own when: clause must still gate the sum type")
}

func TestResolve_AskChannelScopedEventUsesItsOwnAPIBlock(t *testing.T) {
	d := &spec.Descriptor{Events: map[string]spec.EventSpec{
		"permission": {
			API:   &spec.ChannelBlock{Ask: spec.WireRef{"approval/request"}, Map: spec.FieldMap{"tool_name": mapping.Paths("tool")}},
			Hooks: &spec.ChannelBlock{Ask: spec.WireRef{"PermissionRequest"}, Map: spec.FieldMap{"tool_name": mapping.Paths("tool_name")}},
		},
	}}

	canonical, ok := dispatch.Resolve(d, "approval/request", map[string]any{})
	require.True(t, ok, "the api block's own ask: must resolve the wire method")
	assert.Equal(t, "permission", canonical)

	_, ok = dispatch.Resolve(d, "PermissionRequest", map[string]any{})
	assert.False(t, ok, "a channel-scoped event's hooks: wire name must not resolve on the api transport")
}

func decodeParams(t *testing.T, raw string) map[string]any {
	t.Helper()
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &params))
	return params
}

func TestRegression_BothCodexApprovalMethodsResolveToPermission(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	for wire, raw := range map[string]string{
		"item/commandExecution/requestApproval": commandExecutionApproval,
		"item/fileChange/requestApproval":       fileChangeApproval,
	} {
		t.Run(wire, func(t *testing.T) {
			canonical, ok := dispatch.Resolve(d, wire, decodeParams(t, raw))
			require.True(t, ok, "codex asks for approval here and Crowbar must recognise it")
			assert.Equal(t, "permission", canonical)
		})
	}
}

func TestRegression_CodexPermissionsRequestApprovalIsNotTheApprovalWire(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	_, ok := dispatch.Resolve(d, "item/permissions/requestApproval", map[string]any{})

	assert.False(t, ok)
}

func TestRegression_ApprovalRepliesUseCodexsOwnDecisionVocabulary(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	for key, want := range map[string]string{
		"allow": `{"decision":"accept"}`,
		"deny":  `{"decision":"decline"}`,
	} {
		t.Run(key, func(t *testing.T) {
			out, err := answer.Render(d, "permission", []byte(commandExecutionApproval),
				models.AnswerDecision{Key: key, Reason: "no"})

			require.NoError(t, err)
			assert.JSONEq(t, want, string(out))
		})
	}
}

func TestRegression_DenyDeclinesRatherThanCancellingTheTurn(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	out, err := answer.Render(d, "permission", []byte(commandExecutionApproval),
		models.AnswerDecision{Key: "deny"})

	require.NoError(t, err)
	assert.NotContains(t, string(out), "cancel")
}

func TestRegression_ApprovalPayloadCarriesSomethingToShow(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	fields, declared := d.EventFieldsFor("permission", spec.ChannelAPI)
	require.True(t, declared)

	params := decodeParams(t, commandExecutionApproval)

	assert.Equal(t, "01a090df-569c-7393-a274-9d92400ce8eb",
		mapping.String(params, fields["session_id"]))
	assert.Equal(t, "Allow the exact curl command to access example.com outside the restricted sandbox?",
		mapping.String(params, fields["message"]))
	assert.NotEmpty(t, fields["tool_name"], "declaresChoice needs a field, or no card is built at all")
}

func TestResolve_AnInterruptedTurnIsStillAnOrdinaryStop(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	canonical, ok := dispatch.Resolve(d, "turn/completed", map[string]any{
		"threadId": "t",
		"turn":     map[string]any{"id": "tn", "status": "interrupted"},
	})

	require.True(t, ok)
	assert.Equal(t, "turn_stop", canonical)
}

func TestRegression_EveryToolItemVariantRoutesToToolPre(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	for _, variant := range []string{
		"commandExecution", "fileChange", "mcpToolCall", "webSearch", "dynamicToolCall",
	} {
		t.Run(variant, func(t *testing.T) {
			canonical, ok := dispatch.Resolve(d, "item/started", map[string]any{
				"item": map[string]any{"type": variant, "id": "i1"},
			})
			require.True(t, ok, "%s must be recognised as a tool call", variant)
			assert.Equal(t, "tool_pre", canonical)
		})
	}
}

func TestRegression_AFailedToolItemRoutesToToolFail(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	for _, status := range []string{"failed", "declined"} {
		t.Run(status, func(t *testing.T) {
			canonical, ok := dispatch.Resolve(d, "item/completed", map[string]any{
				"item": map[string]any{"type": "commandExecution", "id": "i1", "status": status},
			})
			require.True(t, ok)
			assert.Equal(t, "tool_fail", canonical)
		})
	}
}

func TestResolve_ACompletedToolItemStillRoutesToToolPost(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "item/completed", map[string]any{
		"item": map[string]any{"type": "commandExecution", "id": "i1", "status": "completed"},
	})
	require.True(t, ok)
	assert.Equal(t, "tool_post", canonical)
}

func TestRegression_AStatuslessToolItemStillClosesViaToolPost(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "item/completed", map[string]any{
		"item": map[string]any{"type": "webSearch", "id": "w1", "query": "codex"},
	})
	require.True(t, ok, "a webSearch must still close")
	assert.Equal(t, "tool_post", canonical)
}

func TestResolve_NoThreadStatusIsMapped(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	for _, fixture := range []string{"thread_status_changed.idle", "thread_status_changed"} {
		_, ok := dispatch.Resolve(d, "thread/status/changed", loadParams(t, fixture))
		assert.False(t, ok, "%s must not end a turn", fixture)
	}
	for _, status := range []string{"systemError", "notLoaded"} {
		_, ok := dispatch.Resolve(d, "thread/status/changed", map[string]any{
			"threadId": "t1", "status": map[string]any{"type": status},
		})
		assert.False(t, ok, "%s does not mean the work is done", status)
	}
}

func TestRiders_AFrameDeclaredAlsoCarriesItsDiff(t *testing.T) {
	d, err := descriptor.ParseV0([]byte(`
id: acme
display_name: Acme
runtime:
  transport: api
  api: { protocol: jsonrpc2, serve: [acme, serve] }
  spawn: { cmd: acme }
events:
  tool_post:
    in: tool/done
    map: { tool_id: id }
  diff_update:
    in: tool/done
    also: true
    map: { text: diff }
`))
	require.NoError(t, err)
	params := map[string]any{"id": "t1", "diff": "--- a/f\n+++ b/f\n"}

	primary, ok := dispatch.Resolve(d, "tool/done", params)

	require.True(t, ok)
	assert.Equal(t, "tool_post", primary, "a rider is never the frame's own event")
	assert.Equal(t, []string{"diff_update"}, dispatch.Riders(d, "tool/done", params))
	assert.Empty(t, dispatch.Riders(d, "tool/other", params))
}
