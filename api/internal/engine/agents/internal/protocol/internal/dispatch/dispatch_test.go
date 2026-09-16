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

// Captured verbatim off a real codex-cli 0.149.1 app-server, thread/start'ed with
// sandbox workspace-write + approvalPolicy on-request (Crowbar's `trusted` level),
// asked to run a command needing network access.
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

// Same session shape, asked instead to apply_patch a file outside the writable root.
// codex namespaces this under its OWN method — the whole reason one canonical event
// has to answer to more than one wire name.
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
	d, err := descriptor.ParseV3(raw)
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

func TestResolve_UnknownWireMethodIsNotOK(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "some/method/nobody/declared", map[string]any{})
	assert.False(t, ok)
}

func TestResolve_OutboundEventsAreNeverCandidates(t *testing.T) {
	// "prompt" declares out: turn/start — Resolve must never match an inbound
	// wire method against an outbound event's Send templates.
	d := loadCodexAPIDescriptor(t)
	_, ok := dispatch.Resolve(d, "turn/start", map[string]any{})
	assert.False(t, ok, "turn/start is what WE send, not something codex reports")
}

func TestResolve_AskEventsAreCandidatesToo(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "item/commandExecution/requestApproval", map[string]any{"tool": "shell"})
	require.True(t, ok)
	assert.Equal(t, "permission", canonical)
}

func TestResolve_WhenClauseThatDoesNotMatchFallsThrough(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	// item/started with an item.type this descriptor's when: clauses do not
	// list at all (neither user_prompt's userMessage nor tool_pre's set).
	_, ok := dispatch.Resolve(d, "item/started", map[string]any{
		"item": map[string]any{"type": "reasoning"},
	})
	assert.False(t, ok)
}

// turn/completed is a sum type on turn.status. Ungated, turn_stop swallowed the
// failure case too: a failed turn was ingested as an ordinary stop whose message
// resolved to nothing, so the chat ended on an empty assistant reply and
// turn.error.message was discarded.
func TestRegression_AFailedTurnRoutesToTurnFailedNotTurnStop(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	params := loadParams(t, "turn_completed.failed-schema")
	turn, _ := params["turn"].(map[string]any)
	require.Equal(t, "failed", turn["status"], "fixture must carry the discriminator")

	canonical, ok := dispatch.Resolve(d, "turn/completed", params)

	require.True(t, ok)
	assert.Equal(t, "turn_failed", canonical)
}

func decodeParams(t *testing.T, raw string) map[string]any {
	t.Helper()
	var params map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &params))
	return params
}

// The descriptor named item/permissions/requestApproval, which codex 0.149.1 never
// sends: it asks under item/commandExecution/requestApproval for a command and
// item/fileChange/requestApproval for a patch. Resolve matches the wire method
// exactly, so neither resolved, no permission card was ever raised, and the CLI
// blocked on a reply that could not arrive — a turn measured sitting
// waitingOnApproval with its tool still "running" until the approval budget expired.
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

// item/permissions/requestApproval does not fire on 0.149.1 (not even with
// --enable exec_permission_approvals) and its response carries a
// GrantedPermissionProfile rather than a decision, so it can never be the wire an
// allow/deny reply is written back to.
func TestRegression_CodexPermissionsRequestApprovalIsNotTheApprovalWire(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	_, ok := dispatch.Resolve(d, "item/permissions/requestApproval", map[string]any{})

	assert.False(t, ok)
}

// `approved`/`denied` are not in codex's decision enum. Live, both came back
// "unknown variant ... expected one of accept, acceptForSession,
// acceptWithExecpolicyAmendment, applyNetworkPolicyAmendment, decline, cancel" and
// codex failed the gated tool call with Rejected("approval request failed") — so an
// APPROVE broke the turn exactly as a deny did.
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

// Deny must refuse THIS call and leave the agent free to carry on. codex spells that
// `decline`; `cancel` is the other denial it offers and it interrupts the whole turn,
// which is the Stop button's job and not a permission card's.
func TestRegression_DenyDeclinesRatherThanCancellingTheTurn(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	out, err := answer.Render(d, "permission", []byte(commandExecutionApproval),
		models.AnswerDecision{Key: "deny"})

	require.NoError(t, err)
	assert.NotContains(t, string(out), "cancel")
}

// A card only exists if the choice does: inbound builds one when the descriptor
// declares any of prompt_id/tool_name/tool_input/questions/suggestions, and codex's
// own sentence for what it wants is what the reader decides on.
func TestRegression_ApprovalPayloadCarriesSomethingToShow(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	fields, declared := d.EventFields("permission")
	require.True(t, declared)

	params := decodeParams(t, commandExecutionApproval)

	assert.Equal(t, "01a090df-569c-7393-a274-9d92400ce8eb",
		mapping.String(params, fields["session_id"]))
	assert.Equal(t, "Allow the exact curl command to access example.com outside the restricted sandbox?",
		mapping.String(params, fields["message"]))
	assert.NotEmpty(t, fields["tool_name"], "declaresChoice needs a field, or no card is built at all")
}

// An interrupt is a stop the human asked for, not a failure.
func TestResolve_AnInterruptedTurnIsStillAnOrdinaryStop(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	canonical, ok := dispatch.Resolve(d, "turn/completed", map[string]any{
		"threadId": "t",
		"turn":     map[string]any{"id": "tn", "status": "interrupted"},
	})

	require.True(t, ok)
	assert.Equal(t, "turn_stop", canonical)
}

// Every ThreadItem variant Crowbar treats as a tool must route to tool_pre on
// item/started. webSearch and dynamicToolCall were missing from the when: gate, so
// a turn that searched the web showed no activity at all for that step.
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

// tool_fail and tool_post share one wire method and are separated only by
// item.status. dispatch.Resolve tries candidates in sorted name order, so tool_fail
// is offered first and its narrower when: wins.
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

// webSearch carries NO status field, and mapping.Match refuses to match a clause
// whose path is missing. Had tool_post been gated on item.status the way tool_fail
// is, every web search would have stayed open forever.
func TestRegression_AStatuslessToolItemStillClosesViaToolPost(t *testing.T) {
	d := loadCodexAPIDescriptor(t)
	canonical, ok := dispatch.Resolve(d, "item/completed", map[string]any{
		"item": map[string]any{"type": "webSearch", "id": "w1", "query": "codex"},
	})
	require.True(t, ok, "a webSearch must still close")
	assert.Equal(t, "tool_post", canonical)
}

// thread/status/changed is a sum type on status.type. Only `idle` means the work
// is finished — `active` is a turn opening, and `systemError`/`notLoaded` mean
// broken or not-yet-loaded, neither of which may close a turn.
func TestResolve_OnlyTheIdleThreadStatusIsMapped(t *testing.T) {
	d := loadCodexAPIDescriptor(t)

	canonical, ok := dispatch.Resolve(d, "thread/status/changed",
		loadParams(t, "thread_status_changed.idle"))
	require.True(t, ok)
	assert.Equal(t, "idle", canonical)

	// The live capture of the ACTIVE variant, which must map to nothing.
	_, ok = dispatch.Resolve(d, "thread/status/changed",
		loadParams(t, "thread_status_changed"))
	assert.False(t, ok, "an opening turn must never be read as a finished one")

	for _, status := range []string{"systemError", "notLoaded"} {
		_, ok := dispatch.Resolve(d, "thread/status/changed", map[string]any{
			"threadId": "t1", "status": map[string]any{"type": status},
		})
		assert.False(t, ok, "%s does not mean the work is done", status)
	}
}
