package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// mapTurnStop maps a raw claude Stop payload through the real shipped descriptor —
// no hand-built fixture — so these tests pin the DESCRIPTOR's field path as well as
// the engine's counting.
func mapTurnStop(t *testing.T, providerID string, rawPayload string) engineagent.CanonicalEvent {
	t.Helper()
	// The REAL shipped descriptor (t.TempDir has no override), so these pin the
	// descriptor's own field path, not a fixture that could drift from it.
	d, err := engineagent.ResolveDescriptor(t.TempDir(), providerID)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(rawPayload), &payload))

	ev, err := d.MapHook("turn_stop", payload)
	require.NoError(t, err)
	return ev
}

// TestClaudeTurnStop_CountsBackgroundTasks is the fix, at the descriptor seam. The
// payload is a VERBATIM slice of a real claude 2.1.212 Stop hook captured while a
// background subagent was running — the exact moment the spinner used to go dark.
//
// The count is what keeps it spinning, and it comes from the CLI's own restated list,
// not from anything Crowbar tallies.
func TestClaudeTurnStop_CountsBackgroundTasks(t *testing.T) {
	ev := mapTurnStop(t, "claude", `{
		"session_id": "0929f0ca-9341-4193-a96d-5c3e6985bbd5",
		"hook_event_name": "Stop",
		"last_assistant_message": "Launched. The subagent is running in the background.",
		"background_tasks": [
			{"id":"abbe4333c2384e2dc","type":"subagent","status":"running",
			 "description":"Run sleep and echo command","agent_type":"general-purpose"}
		],
		"session_crons": []
	}`)

	assert.Equal(t, 1, ev.AsyncWork,
		"a Stop reporting one running background task must map to one unit of async work")
	assert.Equal(t, "0929f0ca-9341-4193-a96d-5c3e6985bbd5", ev.SessionID)
	assert.Equal(t, "Launched. The subagent is running in the background.", ev.Message)
}

// TestClaudeTurnStop_EmptyBackgroundTasksIsIdle is the other half of the level: claude
// REMOVES entries as their work finishes, so the empty list is how "done" is reported —
// the observed [running,running] → [] transition. Without this the spinner never stops.
func TestClaudeTurnStop_EmptyBackgroundTasksIsIdle(t *testing.T) {
	ev := mapTurnStop(t, "claude", `{
		"session_id": "s1",
		"last_assistant_message": "The subagent finished.",
		"background_tasks": [],
		"session_crons": []
	}`)

	assert.Equal(t, 0, ev.AsyncWork, "an empty background_tasks is the CLI reporting it is done")
}

// TestClaudeTurnStop_CountsEveryTaskRegardlessOfStatus pins that the level is the LIST
// LENGTH and nothing cleverer. Crowbar does not read `status`, and must not: statuses are
// claude's private vocabulary, and the engine that counts them is shared by every
// provider. Entries leave the list when they are done — that is the contract.
func TestClaudeTurnStop_CountsEveryTaskRegardlessOfStatus(t *testing.T) {
	ev := mapTurnStop(t, "claude", `{
		"session_id": "s1",
		"last_assistant_message": "working",
		"background_tasks": [
			{"id":"a","type":"subagent","status":"running"},
			{"id":"b","type":"subagent","status":"queued"},
			{"id":"c","type":"subagent","status":"whatever-claude-invents-next"}
		]
	}`)

	assert.Equal(t, 3, ev.AsyncWork)
}

// TestClaudeTurnStop_IgnoresSessionCrons is a REAL trap, not a hypothetical one: the same
// Stop payload carries session_crons, and a cron is a SCHEDULE, not work in flight.
// Counting it would leave the spinner running for the life of the session — a permanent
// lie, which is worse than the bug being fixed.
func TestClaudeTurnStop_IgnoresSessionCrons(t *testing.T) {
	ev := mapTurnStop(t, "claude", `{
		"session_id": "s1",
		"last_assistant_message": "scheduled",
		"background_tasks": [],
		"session_crons": [{"id":"cron-1","schedule":"*/5 * * * *"}]
	}`)

	assert.Equal(t, 0, ev.AsyncWork, "a cron is a schedule, not async work in flight")
}

// TestClaudeTurnStop_MissingBackgroundTasksDegradesToTurnOnly is the safe-degradation
// contract, and it needs no version gate: an OLDER claude whose Stop payload has no
// background_tasks key reports 0, which folds Working back to the turn alone — exactly
// the behaviour before any of this existed.
func TestClaudeTurnStop_MissingBackgroundTasksDegradesToTurnOnly(t *testing.T) {
	ev := mapTurnStop(t, "claude", `{"session_id":"s1","last_assistant_message":"done"}`)

	assert.Equal(t, 0, ev.AsyncWork)
	assert.Equal(t, "s1", ev.SessionID, "the rest of the mapping must be unaffected")
}

// TestClaudeTurnStop_MalformedBackgroundTasksIsIdle: a non-array under the mapped path
// is nonsense we must not guess at. 0 is the only safe reading — the alternative is a
// spinner stuck on because a payload changed shape.
func TestClaudeTurnStop_MalformedBackgroundTasksIsIdle(t *testing.T) {
	for name, raw := range map[string]string{
		"string": `{"session_id":"s1","last_assistant_message":"x","background_tasks":"nope"}`,
		"object": `{"session_id":"s1","last_assistant_message":"x","background_tasks":{"a":1}}`,
		"null":   `{"session_id":"s1","last_assistant_message":"x","background_tasks":null}`,
		"number": `{"session_id":"s1","last_assistant_message":"x","background_tasks":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, 0, mapTurnStop(t, "claude", raw).AsyncWork,
				"a background_tasks that is not a list must read as idle, never as stuck-on")
		})
	}
}

// TestCodexTurnStop_ReportsNoAsyncWork is the PROVIDER-AGNOSTIC requirement, enforced
// against the real codex descriptor: it maps no async_work, so even a payload that
// happens to carry a background_tasks array reports 0 and codex keeps its turn-only
// behaviour bit-identical.
//
// The engine learns what to count from the DESCRIPTOR. It knows nothing about subagents.
func TestCodexTurnStop_ReportsNoAsyncWork(t *testing.T) {
	ev := mapTurnStop(t, "codex", `{
		"session_id": "s1",
		"last_assistant_message": "done",
		"background_tasks": [{"id":"a"},{"id":"b"}]
	}`)

	assert.Equal(t, 0, ev.AsyncWork,
		"codex maps no async_work: an unmapped field must never be counted")
	assert.Equal(t, "s1", ev.SessionID)
	assert.Equal(t, "done", ev.Message)
}

// TestCodexDescriptor_MapsNoAsyncWorkAtAll guards the descriptor itself rather than one
// payload: codex's ledger must show only created/title_set/turn_started/turn_stopped, and
// that starts with it never opting into the async-work vocabulary anywhere.
func TestCodexDescriptor_MapsNoAsyncWorkAtAll(t *testing.T) {
	d, err := engineagent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)

	for kind, fields := range d.Hooks.Events {
		_, mapped := fields["async_work"]
		assert.Falsef(t, mapped, "codex must map no async_work (found on %q)", kind)
	}
}
