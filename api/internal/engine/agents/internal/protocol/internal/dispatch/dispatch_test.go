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
	raw, err := os.ReadFile("../descriptor/descriptors-v3/experimental/codex-api.yaml")
	require.NoError(t, err)
	d, err := descriptor.ParseV3(raw)
	require.NoError(t, err)
	return d
}

func loadParams(t *testing.T, fixture string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/fixtures/codex-api/" + fixture + ".json")
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
