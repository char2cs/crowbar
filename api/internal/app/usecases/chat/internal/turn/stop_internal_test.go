package turn

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
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

func newStopTestTurns(t *testing.T) (*Turns, *fakeStopActivity) {
	t.Helper()
	activity := &fakeStopActivity{}
	return New(Deps{Activity: activity}), activity
}

func TestRecordStop_RecordsAndResolvesAStoppedInterruption(t *testing.T) {
	turns, activity := newStopTestTurns(t)

	err := turns.RecordStop(context.Background(), "chat-1")

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
	turns, activity := newStopTestTurns(t)
	require.NoError(t, turns.RecordStop(context.Background(), "chat-1"))
	require.NoError(t, turns.RecordStop(context.Background(), "chat-1"))

	require.Len(t, activity.interrupts, 2)
	assert.NotEqual(t, activity.interrupts[0].id, activity.interrupts[1].id)
}
