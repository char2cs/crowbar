package spec_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// channelSplitYAML mirrors the design spec's own tool_pre example
// (docs/plans/2026-09-22-descriptor-channel-split.md, 2.1): one canonical
// event, two channel blocks, each with its own wire name and map.
const channelSplitYAML = `
events:
  tool_pre:
    required: [session_id, tool_id, tool_name]
    api:
      in: item/started
      when: { item.type: { any_of: [commandExecution, fileChange] } }
      map: { session_id: threadId, tool_id: item.id }
      fixtures: [item-started.commandExecution.json]
    hooks:
      in: PreToolUse
      map: { session_id: session_id, tool_id: tool_use_id }
      fixtures: [pre-tool-use.bash.json]
`

func parseChannelSplit(t *testing.T) spec.Descriptor {
	t.Helper()
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(channelSplitYAML), &d))
	return d
}

func TestEventSpec_ChannelBlocksParse(t *testing.T) {
	d := parseChannelSplit(t)
	ev := d.Events["tool_pre"]

	require.NotNil(t, ev.API)
	require.NotNil(t, ev.Hooks)
	assert.Equal(t, spec.WireRef{"item/started"}, ev.API.In)
	assert.Equal(t, []string{"commandExecution", "fileChange"}, ev.API.When["item.type"])
	assert.Equal(t, []string{"threadId"}, ev.API.Map["session_id"])
	assert.Equal(t, []string{"item-started.commandExecution.json"}, ev.API.Fixtures)

	assert.Equal(t, spec.WireRef{"PreToolUse"}, ev.Hooks.In)
	assert.Equal(t, []string{"session_id"}, ev.Hooks.Map["session_id"])
	assert.Equal(t, []string{"pre-tool-use.bash.json"}, ev.Hooks.Fixtures)

	assert.Equal(t, []string{"session_id", "tool_id", "tool_name"}, ev.Required)
	assert.True(t, ev.HasChannelBlocks())
}

func TestEventSpec_WireEventForSelectsTheBlockByChannel(t *testing.T) {
	d := parseChannelSplit(t)
	ev := d.Events["tool_pre"]

	apiWire, apiDir := ev.WireEventFor(spec.ChannelAPI)
	assert.Equal(t, spec.WireRef{"item/started"}, apiWire)
	assert.Equal(t, "in", apiDir)

	hooksWire, hooksDir := ev.WireEventFor(spec.ChannelHooks)
	assert.Equal(t, spec.WireRef{"PreToolUse"}, hooksWire)
	assert.Equal(t, "in", hooksDir)
}

func TestEventSpec_WhenForSelectsTheBlockByChannel(t *testing.T) {
	d := parseChannelSplit(t)
	ev := d.Events["tool_pre"]

	assert.Equal(t, []string{"commandExecution", "fileChange"}, ev.WhenFor(spec.ChannelAPI)["item.type"])
	assert.Empty(t, ev.WhenFor(spec.ChannelHooks), "the hooks block declares no when:")
}

// A legacy event (no channel blocks at all) answers the SAME on every
// channel — the whole compatibility guarantee: P2 must not change behaviour
// for an event that has not opted into the split.
func TestEventSpec_LegacyEventAnswersTheSameOnEveryChannel(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  turn_stop:
    in: turn/completed
    when: { turn.status: completed }
    map: { message: turn.last }
`), &d))
	ev := d.Events["turn_stop"]

	require.False(t, ev.HasChannelBlocks())
	for _, ch := range []spec.Channel{spec.ChannelAPI, spec.ChannelHooks, spec.Channel("")} {
		wire, dir := ev.WireEventFor(ch)
		assert.Equal(t, spec.WireRef{"turn/completed"}, wire, "channel %q", ch)
		assert.Equal(t, "in", dir, "channel %q", ch)
		assert.Equal(t, []string{"completed"}, ev.WhenFor(ch)["turn.status"], "channel %q", ch)
	}
}

// A channel-scoped event declaring only ONE block is absent on the other —
// EventFieldsFor's ok=false is what lets the caller refuse it with a named
// error instead of silently mapping to nothing (design spec 2.1).
func TestEventSpec_AChannelTheEventDoesNotDeclareIsAbsent(t *testing.T) {
	d := parseChannelSplit(t)
	ev := d.Events["tool_pre"]

	wire, dir := ev.WireEventFor(spec.Channel("oneshot"))
	assert.Empty(t, wire)
	assert.Empty(t, dir)

	_ = d // EventFieldsFor is exercised in TestDescriptor_EventFieldsFor below
}

func TestDescriptor_EventFieldsFor(t *testing.T) {
	d := parseChannelSplit(t)

	apiFields, ok := d.EventFieldsFor("tool_pre", spec.ChannelAPI)
	require.True(t, ok)
	assert.Equal(t, []string{"threadId"}, apiFields["session_id"])
	assert.Equal(t, []string{"item.id"}, apiFields["tool_id"])

	hooksFields, ok := d.EventFieldsFor("tool_pre", spec.ChannelHooks)
	require.True(t, ok)
	assert.Equal(t, []string{"session_id"}, hooksFields["session_id"])
	assert.Equal(t, []string{"tool_use_id"}, hooksFields["tool_id"])

	_, ok = d.EventFieldsFor("tool_pre", spec.Channel("oneshot"))
	assert.False(t, ok, "a channel this event declares no block for is absent, not silently mapped")

	_, ok = d.EventFieldsFor("never_declared", spec.ChannelAPI)
	assert.False(t, ok, "an undeclared event is absent regardless of channel")
}

func TestDescriptor_EventFieldsForFallsBackToTheLegacyMapOnEveryChannel(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  session_start:
    in: session_start
    map: { session_id: session_id }
`), &d))

	for _, ch := range []spec.Channel{spec.ChannelAPI, spec.ChannelHooks} {
		fields, ok := d.EventFieldsFor("session_start", ch)
		require.True(t, ok, "channel %q", ch)
		assert.Equal(t, []string{"session_id"}, fields["session_id"], "channel %q", ch)
	}
}

func TestDescriptor_EventFixturesFallsBackToTheLegacyListOnEveryChannel(t *testing.T) {
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(`
events:
  session_start:
    in: session_start
    map: { session_id: session_id }
    fixtures: [SessionStart.json]
`), &d))

	for _, ch := range []spec.Channel{spec.ChannelAPI, spec.ChannelHooks} {
		assert.Equal(t, []string{"SessionStart.json"}, d.EventFixtures("session_start", ch), "channel %q", ch)
	}
	assert.Empty(t, d.EventFixtures("never_declared", spec.ChannelAPI))
}

func TestDescriptor_EventRequiredAndFixturesAreExposed(t *testing.T) {
	d := parseChannelSplit(t)

	assert.Equal(t, []string{"session_id", "tool_id", "tool_name"}, d.EventRequired("tool_pre"))
	assert.Equal(t, []string{"item-started.commandExecution.json"}, d.EventFixtures("tool_pre", spec.ChannelAPI))
	assert.Equal(t, []string{"pre-tool-use.bash.json"}, d.EventFixtures("tool_pre", spec.ChannelHooks))
	assert.Empty(t, d.EventFixtures("tool_pre", spec.Channel("oneshot")))
	assert.Empty(t, d.EventRequired("never_declared"))
}

func TestEventSpec_AnyWireEventFindsAChannelBlocksWireName(t *testing.T) {
	d := parseChannelSplit(t)
	ev := d.Events["tool_pre"]

	wire, dir := ev.AnyWireEvent()
	assert.False(t, wire.Empty())
	assert.Equal(t, "in", dir)
}

// askChannelSplitYAML mirrors permission's real shape (codex.yaml, since
// docs/plans/2026-09-22-descriptor-channel-split.md P3's own gap report): an
// ASK-direction event, channel-split, each block naming its own wire method(s)
// under ask: rather than in:. Reply stays flat on the event — see
// ChannelBlock's own doc comment on why it is not per-block.
const askChannelSplitYAML = `
events:
  permission:
    required: [session_id, tool_name]
    timeout_seconds: 270
    api:
      ask: [item/commandExecution/requestApproval, item/fileChange/requestApproval]
      map: { session_id: threadId, tool_name: tool }
    hooks:
      ask: PermissionRequest
      map: { session_id: session_id, tool_name: tool_name }
    reply:
      allow: '{"decision":"accept"}'
      deny:  '{"decision":"decline"}'
`

func parseAskChannelSplit(t *testing.T) spec.Descriptor {
	t.Helper()
	var d spec.Descriptor
	require.NoError(t, yaml.Unmarshal([]byte(askChannelSplitYAML), &d))
	return d
}

func TestEventSpec_AskChannelBlocksParse(t *testing.T) {
	d := parseAskChannelSplit(t)
	ev := d.Events["permission"]

	require.NotNil(t, ev.API)
	require.NotNil(t, ev.Hooks)
	assert.Equal(t, spec.WireRef{
		"item/commandExecution/requestApproval", "item/fileChange/requestApproval",
	}, ev.API.Ask)
	assert.Equal(t, spec.WireRef{"PermissionRequest"}, ev.Hooks.Ask)
	assert.True(t, ev.API.In.Empty(), "an ask block declares ask:, not in:")
}

// TestEventSpec_WireEventForResolvesAskDirectionFromChannelBlock is the
// direct reproduction of the P3 gap report: "WireEventFor always returns
// direction 'in' for a channel-scoped event" — an ask-direction channel block
// must report direction "ask" and its own wire name, on EACH channel.
func TestEventSpec_WireEventForResolvesAskDirectionFromChannelBlock(t *testing.T) {
	d := parseAskChannelSplit(t)
	ev := d.Events["permission"]

	apiWire, apiDir := ev.WireEventFor(spec.ChannelAPI)
	assert.Equal(t, spec.WireRef{
		"item/commandExecution/requestApproval", "item/fileChange/requestApproval",
	}, apiWire)
	assert.Equal(t, "ask", apiDir)

	hooksWire, hooksDir := ev.WireEventFor(spec.ChannelHooks)
	assert.Equal(t, spec.WireRef{"PermissionRequest"}, hooksWire)
	assert.Equal(t, "ask", hooksDir)
}

func TestEventSpec_AnyWireEventFindsAnAskChannelBlock(t *testing.T) {
	d := parseAskChannelSplit(t)
	ev := d.Events["permission"]

	wire, dir := ev.AnyWireEvent()
	assert.False(t, wire.Empty())
	assert.Equal(t, "ask", dir)
}
