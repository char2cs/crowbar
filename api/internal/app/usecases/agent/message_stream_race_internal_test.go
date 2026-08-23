package agent

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
)

const raceIncrements = 3000

type raceChats struct {
	agentchat.EventStore
}

func (raceChats) GetChat(
	_ context.Context,
	id string,
) (domain.AgentChat, error) {
	return domain.AgentChat{ID: id}, nil
}

func (raceChats) AbandonTurn(
	_ context.Context,
	chatID string,
	_ time.Time,
) (domain.AgentChat, error) {
	return domain.AgentChat{ID: chatID}, nil
}

type raceRunners struct {
	agentrunner.EventStore
}

func (raceRunners) LiveRunnerForChat(
	_ context.Context,
	_ string,
) (domain.AgentRunner, error) {
	return domain.AgentRunner{ID: "runner-1", ProviderID: "claude"}, nil
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
	usecase *turnUsecase
	done    chan struct{}
}

func (r *streamRacer) stream() {
	defer close(r.done)
	for i := range raceIncrements {
		r.usecase.messages.observe("c", "t", "m", i, false, "increment", time.Now())
	}
}

func (r *streamRacer) sweep(t *testing.T) {
	t.Helper()
	for !r.finished() {
		_, err := r.usecase.AbandonMessage(context.Background(), "c")
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

func TestMessageStreams_TheSweepAbandonsWhileAHookIsStillStreaming(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	slog.SetDefault(slog.New(slog.DiscardHandler))

	racer := &streamRacer{
		usecase: &turnUsecase{
			chats:    raceChats{},
			runners:  raceRunners{},
			activity: raceActivity{},
			messages: newMessageStreams(),
			work:     newChatWorkStates(),
		},
		done: make(chan struct{}),
	}

	var both sync.WaitGroup
	both.Add(2)
	go func() {
		defer both.Done()
		racer.stream()
	}()
	go func() {
		defer both.Done()
		racer.sweep(t)
	}()
	both.Wait()
}
