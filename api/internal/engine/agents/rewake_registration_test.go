package agents_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// This file holds the SHIPPED claude spawn to its side of the rewake bargain.
//
// Everything else about this channel is Crowbar's own machinery and is tested as
// such. This is the one seam where Crowbar has to say something to the provider
// and be understood, and it is the seam where a silent break looks like a working
// feature: get the registration wrong and delivery simply never happens — the
// daemon falls back to a restart, every prompt still lands, and nobody notices
// the optimisation is dead. So the registration is asserted rather than assumed.
//
// Every claim below was measured against claude 2.1.235 on 2026-08-18 by reading
// the hook schema out of the binary and running the channel end to end on a live
// interactive PTY.
func claudeSettings(t *testing.T) map[string]any {
	t.Helper()
	a := get(t, "claude")
	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(),
		Segid: "SEG", RunnerToken: "TOKEN", Provider: "claude",
		ProjectID: "P", RepoID: "R", WorkspaceID: "W",
		CrowbarHook: "/bin/crowbar",
	}
	plan, err := a.SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)
	t.Cleanup(plan.Cleanup)

	body, err := os.ReadFile(filepath.Join(ctx.Tmp, "settings.json"))
	require.NoError(t, err, "the descriptor writes its hook registration here")

	var settings map[string]any
	require.NoError(t, json.Unmarshal(body, &settings),
		"the sentinel and summary are interpolated INTO this JSON; a quote in either would break it here")
	return settings
}

// stopHooks returns the Stop matcher's hook list from a rendered settings file.
func stopHooks(t *testing.T, settings map[string]any) []map[string]any {
	t.Helper()
	hooks, ok := settings["hooks"].(map[string]any)
	require.True(t, ok)
	matchers, ok := hooks["Stop"].([]any)
	require.True(t, ok)
	require.Len(t, matchers, 1)
	entry, ok := matchers[0].(map[string]any)
	require.True(t, ok)
	list, ok := entry["hooks"].([]any)
	require.True(t, ok)
	out := make([]map[string]any, 0, len(list))
	for _, h := range list {
		hook, hookOK := h.(map[string]any)
		require.True(t, hookOK)
		out = append(out, hook)
	}
	return out
}

// TestClaudeSpawn_ArmsThePromptCollectorWhenATurnEnds is the registration itself.
//
// asyncRewake is what makes a background hook WAKE the model on the blocking exit
// status, and async beside it is not redundant: asyncRewake alone is conditional
// inside claude, and a collector that ever ran in the foreground would block the
// end of every turn for its whole timeout.
func TestClaudeSpawn_ArmsThePromptCollectorWhenATurnEnds(t *testing.T) {
	hooks := stopHooks(t, claudeSettings(t))
	require.Len(t, hooks, 2, "the ordinary turn_stop observation, plus the prompt collector")

	var collector map[string]any
	for _, hook := range hooks {
		if strings.Contains(hook["command"].(string), "await-prompt") {
			collector = hook
		}
	}
	require.NotNil(t, collector, "a turn ending is what arms the collector")
	assert.Equal(t, true, collector["asyncRewake"], "without this the hook cannot wake the model")
	assert.Equal(t, true, collector["async"], "and without this the wake is conditional")
	assert.NotEmpty(t, collector["timeout"], "an unbounded collector is an unbounded orphan")
}

// TestClaudeSpawn_TheWakeStatusReachesTheCollectorFromTheDescriptor keeps the one
// number in this protocol out of Go.
//
// The status a provider takes a collected message on is ITS protocol, not
// Crowbar's. Hardcoded it would be provider knowledge in code, and the next CLI
// to grow a wake mechanism with a different status would need a daemon build to
// use it. Declared, it travels descriptor → argv → collector, and this asserts
// the whole path.
func TestClaudeSpawn_TheWakeStatusReachesTheCollectorFromTheDescriptor(t *testing.T) {
	a := get(t, "claude")
	declared := agents.RewakeWakeStatus(a)
	require.Positive(t, declared, "a status of 0 is success, which is the one status a provider ignores")

	var command string
	for _, hook := range stopHooks(t, claudeSettings(t)) {
		if c, ok := hook["command"].(string); ok && strings.Contains(c, "await-prompt") {
			command = c
		}
	}
	require.NotEmpty(t, command)
	assert.Contains(t, command, fmt.Sprintf("--wake-status %d", declared),
		"the collector is told the status rather than assuming one")
}

// TestClaudeSpawn_RegistersTheSameSentinelItLaterMatchesOn is the drift guard, and
// it is the one that matters most.
//
// The sentinel Crowbar registers here is the ONLY thing that later distinguishes a
// message the user typed from a report the provider's own harness wrote — both
// arrive on one event, from one process, inside markup that opens identically. Two
// spellings of it is the way this feature breaks silently: delivery keeps working,
// and every prompt is filed under the wrong author.
func TestClaudeSpawn_RegistersTheSameSentinelItLaterMatchesOn(t *testing.T) {
	a := get(t, "claude")
	sentinel := agents.RewakeSentinel(a)
	summary := agents.RewakeSummary(a)
	require.NotEmpty(t, sentinel)
	require.NotEmpty(t, summary)

	var collector map[string]any
	for _, hook := range stopHooks(t, claudeSettings(t)) {
		if _, ok := hook["rewakeMessage"]; ok {
			collector = hook
		}
	}
	require.NotNil(t, collector)
	assert.Equal(t, sentinel, collector["rewakeMessage"],
		"the string registered with the provider and the string matched on ingest are one fact")
	assert.Equal(t, summary, collector["rewakeSummary"])

	// And the round trip: the wrapper claude builds from exactly these two values
	// unwraps back to what the user typed.
	wrapped := "<task-notification>\n<summary>" + summary + "</summary>\n</task-notification>\n" +
		"<system-reminder>\n" + sentinel + " say only ACK\n</system-reminder>"
	got, ok := agents.MatchRewakePrompt(a, wrapped)
	require.True(t, ok, "the registered sentinel must satisfy the descriptor's own strip pattern")
	assert.Equal(t, "say only ACK", got)
}

// TestClaudeSpawn_TheCollectorCarriesACredentialNotJustASegmentID: this is the one
// callback that reads, so it cannot be authorised by an id the chats API publishes.
func TestClaudeSpawn_TheCollectorCarriesACredentialNotJustASegmentID(t *testing.T) {
	var command string
	for _, hook := range stopHooks(t, claudeSettings(t)) {
		if c, ok := hook["command"].(string); ok && strings.Contains(c, "await-prompt") {
			command = c
		}
	}
	require.NotEmpty(t, command)
	assert.Contains(t, command, "--token TOKEN")
	assert.Contains(t, command, "--segment SEG")
}

// TestCodexSpawn_ArmsNoCollector is the other provider's whole story. Its hooks
// only run while it is already busy, so an idle session cannot be woken and there
// is nothing here to register — the floor is not a degraded mode for codex, it is
// the mode.
func TestCodexSpawn_ArmsNoCollector(t *testing.T) {
	a := get(t, "codex")
	assert.Empty(t, agents.RewakeSentinel(a))
	assert.Empty(t, agents.RewakeSummary(a))
	assert.Zero(t, agents.RewakeWakeStatus(a),
		"a provider that declares nothing must not be handed a status it never named")
	assert.Equal(t, agents.DeliveryRestartTUI, a.Capabilities().Delivery)
	_, matched := agents.MatchRewakePrompt(a, "anything at all")
	assert.False(t, matched, "and no prompt of its can ever be read as a Crowbar delivery")

	ctx := agents.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(),
		Segid: "SEG", RunnerToken: "TOKEN", Provider: "codex",
		CrowbarHook: "/bin/crowbar",
	}
	plan, err := a.SpawnPlan(ctx, nil, nil)
	require.NoError(t, err)
	t.Cleanup(plan.Cleanup)

	entries, err := os.ReadDir(ctx.Tmp)
	require.NoError(t, err)
	for _, entry := range entries {
		body, readErr := os.ReadFile(filepath.Join(ctx.Tmp, entry.Name()))
		require.NoError(t, readErr)
		assert.NotContains(t, string(body), "await-prompt",
			"codex has no equivalent channel and must be registered for none")
	}
}
