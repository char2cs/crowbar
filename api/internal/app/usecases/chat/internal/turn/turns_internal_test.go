package turn

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/turn/internal/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
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

func (raceChats) AbandonTurn(
	_ context.Context,
	chatID string,
	_ time.Time,
) (domain.Chat, error) {
	return domain.Chat{ID: chatID}, nil
}

type raceRunners struct {
	agentrunner.EventStore
}

func (raceRunners) LiveRunnerForChat(
	_ context.Context,
	_ string,
) (agents.Runner, error) {
	return agents.Runner{ID: "runner-1", ProviderID: "claude"}, nil
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
		r.messages.Observe("c", "t", "m", i, false, "increment", time.Now())
	}
}

func (r *streamRacer) sweep(t *testing.T) {
	t.Helper()
	for !r.finished() {
		_, err := r.turns.AbandonMessage(context.Background(), "c")
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
