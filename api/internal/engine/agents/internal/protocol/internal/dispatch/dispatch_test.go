package dispatch_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/dispatch"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

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
	canonical, ok := dispatch.Resolve(d, "item/permissions/requestApproval", map[string]any{"tool": "shell"})
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
