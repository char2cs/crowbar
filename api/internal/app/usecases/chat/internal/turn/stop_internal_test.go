package turn

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type interruptCall struct {
	chatID, id, kind, detail string
}

// fakeStopActivity records every Interrupt/ResolveInterruption call it
// receives — RecordStop's whole observable effect — and panics on anything
// else via the embedded nil interface, the same shape fakeChoiceActivity uses
// one file over.
type fakeStopActivity struct {
	agentactivity.EventStore

	mu         sync.Mutex
	interrupts []interruptCall
	resolves   []interruptCall
}

func (f *fakeStopActivity) Interrupt(
	_ context.Context, chatID, id, kind, detail string, _ time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.interrupts = append(f.interrupts, interruptCall{chatID, id, kind, detail})
	return nil
}

func (f *fakeStopActivity) ResolveInterruption(
	_ context.Context, chatID, id, kind, detail string, _ time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resolves = append(f.resolves, interruptCall{chatID, id, kind, detail})
	return nil
}

func newStopTestTurns(t *testing.T) (*Turns, *fakeStopActivity, *inflight.Turns) {
	t.Helper()
	activity := &fakeStopActivity{}
	inflightTurns := inflight.NewTurns()
	turns := New(Deps{
		Activity:      activity,
		InflightTurns: inflightTurns,
	})
	return turns, activity, inflightTurns
}

func TestRecordStop_NoOpWhenTheChatIsIdle(t *testing.T) {
	turns, activity, _ := newStopTestTurns(t)

	err := turns.RecordStop(context.Background(), "chat-1", "runner-1")

	require.NoError(t, err)
	assert.Empty(t, activity.interrupts, "an idle chat has no turn to interrupt — StopChat closing a chat tab must stay silent")
	assert.Empty(t, activity.resolves)
}

func TestRecordStop_RecordsAndResolvesAStoppedInterruption_WhenATurnIsInFlight(t *testing.T) {
	turns, activity, inflightTurns := newStopTestTurns(t)
	inflightTurns.Begin("runner-1", "chat-1")

	err := turns.RecordStop(context.Background(), "chat-1", "runner-1")

	require.NoError(t, err)
	require.Len(t, activity.interrupts, 1)
	require.Len(t, activity.resolves, 1)
	opened, closed := activity.interrupts[0], activity.resolves[0]
	assert.Equal(t, "chat-1", opened.chatID)
	assert.Equal(t, engineagents.InterruptStopped, opened.kind)
	assert.Equal(t, opened.id, closed.id,
		"the open and the resolve must name the SAME interruption, or the read side never pairs them into one divider")
	assert.Equal(t, engineagents.InterruptStopped, closed.kind)
}

func TestRecordStop_UsesAFreshIDPerCall_UnlikeCompactionsSharedOne(t *testing.T) {
	// Two stops on the same chat, over its lifetime, are two DIFFERENT
	// interruptions — a person can stop turn one, keep talking, and stop turn
	// two. Compaction's single deterministic id-per-chat would collapse them
	// into one record and lose the second divider entirely.
	turns, activity, inflightTurns := newStopTestTurns(t)
	inflightTurns.Begin("runner-1", "chat-1")
	require.NoError(t, turns.RecordStop(context.Background(), "chat-1", "runner-1"))
	inflightTurns.Begin("runner-1", "chat-1")
	require.NoError(t, turns.RecordStop(context.Background(), "chat-1", "runner-1"))

	require.Len(t, activity.interrupts, 2)
	assert.NotEqual(t, activity.interrupts[0].id, activity.interrupts[1].id)
}

// TestRegression_RecordStopWaitsForAnInFlightHookOnTheSameRunner guards the
// bug reported live 2026-09-12: an "Interrupted" divider landed anchored to a
// message's turn that a self-continuation hook had already superseded only
// 17-31ms earlier — the interrupt was committed AHEAD of replies the CLI had
// already produced by the moment Stop was clicked.
//
// Root cause: interruptTurn's Send only waits for the API connection to say
// the turn is over; it says nothing about whether that SAME turn's own
// closing/reopening hook deliveries have finished being ingested through
// IngestHookDelivery, a wholly separate channel. RecordStop used to commit
// its Interrupt with no regard for a hook still mid-flight for the same
// runner, racing ahead of state that hook was about to (and, chronologically
// at the CLI, already had) supersede.
//
// Proven the same way this repo's own gate_test.go proves the gate itself
// serialises: not with timing (which proves nothing about a race), but by
// making a broken RecordStop a genuine, -race-detectable data race on an
// unsynchronised value — a gate that lets RecordStop in early corrupts
// `order` from two goroutines at once; a gate that waits its turn cannot.
func TestRegression_RecordStopWaitsForAnInFlightHookOnTheSameRunner(t *testing.T) {
	turns, _, inflightTurns := newStopTestTurns(t)
	inflightTurns.Begin("runner-1", "chat-1")

	// Simulates a hook delivery for runner-1 already admitted into
	// IngestHookDelivery and still mid-ingest — holding exactly the gate
	// delivery.go holds across its whole ingest (dedupe, effects, completion).
	release := turns.hookGates.Lock("runner-1")

	var order []string
	done := make(chan struct{})
	go func() {
		defer close(done)
		require.NoError(t, turns.RecordStop(context.Background(), "chat-1", "runner-1"))
		order = append(order, "stop")
	}()

	order = append(order, "hook")
	release()
	<-done

	require.Equal(t, []string{"hook", "stop"}, order,
		"RecordStop must wait for runner-1's own in-flight hook to release its gate before touching "+
			"the activity ledger, or it can anchor the Interrupted divider to state that hook was about "+
			"to supersede")
}
