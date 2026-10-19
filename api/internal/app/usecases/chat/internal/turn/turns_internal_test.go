package turn

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

const raceIncrements = 3000

type raceChats struct {
	agentchat.EventStore
}

func (raceChats) GetChat(
	_ context.Context,
	id string,
) (domain.Chat, error) {
	return domain.Chat{ID: id}, nil
}

// raceRunners is a Runners on which every method panics if reached.
type raceRunners struct {
	Runners
}

type raceActivity struct {
	agentactivity.EventStore
}

func (raceActivity) CloseTurn(
	_ context.Context,
	_ agentactivity.TurnInput,
) error {
	return nil
}

type streamRacer struct {
	turns *Turns
	// messages is the SAME stream the ingress built, held here so the racer can push
	// increments in at the exact seam a hook goroutine does.
	messages *stream.Streams
	done     chan struct{}
}

func (r *streamRacer) stream() {
	defer close(r.done)
	for i := range raceIncrements {
		r.messages.Observe("c", "runner-1", "t", "m", i, true, false, "increment", time.Now())
	}
}

func (r *streamRacer) sweep(t *testing.T) {
	t.Helper()
	runner := agents.Runner{ID: "runner-1", ProviderID: "claude"}
	for !r.finished() {
		_, err := r.turns.AbandonMessageForRunner(context.Background(), "c", runner)
		assert.NoError(t, err)
	}
}

func (r *streamRacer) finished() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

// A7: erasing a chat leaves nothing the turn ingress held for it in memory.
func TestTurns_ForgetChatDropsEveryPerChatEntry(t *testing.T) {
	turns := &Turns{
		live: newLiveText(), messages: stream.New(),
		compacting: newCompactionTurns(), manualCompact: newManualCompactRequests(),
	}
	turns.live.observe("c1", "reasoning", "b1", 0, "thinking")
	turns.messages.Observe("c1", "r1", "t1", "m1", 0, false, false, "partial", time.Unix(100, 0))
	turns.compacting.arm("c1", "turn-compact")
	turns.manualCompact.arm("c1")
	turns.HoldSteer("c1", "req-1", "steer text")

	turns.ForgetChat("c1")

	assert.Empty(t, turns.live.byChat["c1"])
	assert.Empty(t, turns.messages.Unfinished("c1", "r1"))
	assert.False(t, turns.compacting.consume("c1", "turn-compact"))
	assert.False(t, turns.manualCompact.peek("c1"))
	assert.Equal(t, steerText{}, turns.steers.take("c1"))
}

func newTurnLedger(t *testing.T) *ledgerstore.Store {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	l, err := ledgerstore.Open(db, time.Now)
	require.NoError(t, err)
	return l
}
