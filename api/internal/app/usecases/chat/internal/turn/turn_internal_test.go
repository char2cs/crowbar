package turn

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// A compaction's own turn_stop is skipped whole (compaction.go) — but the idle
// report riding with it is about the COMPACTION, not the assistant turn still
// running underneath it. Measured live against codex-cli: idle lands in the
// same millisecond as every turn/completed, so a compaction arms the latch as
// surely as a real close does.
//
// Left armed, the 5s provider-idle sweep (runner/internal/termwait,
// providerSaysItIsIdle) abandons that live turn — Working=false, spinner dark,
// while the CLI is still generating. It is deliberately NOT gated on OpenWork,
// so a running tool call does not save it.
//
// Only codex reaches this: it is the only descriptor mapping idle, and it marks
// no message final, so no mid-turn recordAssistantMessage ever re-opens the
// assistant turn to clear the latch on its own.
func TestRegression_ACompactionsOwnStopStillClearsTheIdleLatch(t *testing.T) {
	turns := New(Deps{})
	turns.compacting.arm("chat-1", "compaction-turn")
	turns.recordIdle(domain.Chat{ID: "chat-1"})

	_, armed := turns.ProviderIdleSince("chat-1")
	require.True(t, armed, "the idle report that rides the compaction's stop arms the latch")

	err := turns.closeTurnFromStop(
		t.Context(),
		domain.Chat{ID: "chat-1"},
		engineagents.Runner{},
		nil,
		engineagents.CanonicalEvent{Kind: "turn_stop", TurnID: "compaction-turn"},
	)
	require.NoError(t, err)

	_, stillArmed := turns.ProviderIdleSince("chat-1")
	require.False(t, stillArmed,
		"a compaction says nothing about the turn running underneath it; leaving the latch armed lets the 5s sweep abandon a live turn")
}

// The failure half of the same sum type takes the identical early return.
func TestRegression_ACompactionsOwnFailureStillClearsTheIdleLatch(t *testing.T) {
	turns := New(Deps{})
	turns.compacting.arm("chat-1", "compaction-turn")
	turns.recordIdle(domain.Chat{ID: "chat-1"})

	err := turns.closeTurnFromFailure(
		t.Context(),
		domain.Chat{ID: "chat-1"},
		engineagents.Runner{},
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnFailed, TurnID: "compaction-turn"},
	)
	require.NoError(t, err)

	_, stillArmed := turns.ProviderIdleSince("chat-1")
	require.False(t, stillArmed, "a failed compaction leaves the same stale latch behind")
}

// foldingChats is a minimal but REAL fold of Working: StopTurn does exactly
// what commands.StopTurn/foldWorking do — Working stays true whenever the
// asyncWork it is handed is positive — so a test against it exercises the
// actual production rule, not a stand-in that always answers one way.
type foldingChats struct {
	agentchat.EventStore
	working bool
}

func (f *foldingChats) StopTurn(
	_ context.Context, chatID string, _ time.Time, asyncWork int,
) (domain.Chat, error) {
	f.working = asyncWork > 0
	return domain.Chat{ID: chatID, Working: f.working, AsyncWork: asyncWork}, nil
}

func (f *foldingChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, Working: f.working}, nil
}

// openWaitActivity answers OpenWork's own two reads (ToolCalls, Subagents)
// with one still-Running "wait" call — codex's real shape for a live
// subagent delegation — and no-ops CloseTurn, which closeAssistantTurn calls
// on its way to recording the reply text this test's event carries.
type openWaitActivity struct {
	agentactivity.EventStore
}

func (openWaitActivity) ToolCalls(
	context.Context, string, int64, int,
) ([]domain.ActivityToolCall, error) {
	return []domain.ActivityToolCall{{ID: "wait-1", Name: "wait", Status: domain.ToolStatusRunning}}, nil
}

func (openWaitActivity) Subagents(context.Context, string) ([]domain.ActivitySubagent, error) {
	return nil, nil
}

func (openWaitActivity) CloseTurn(context.Context, agentactivity.TurnInput) error { return nil }

// noopRunners satisfies closeTurnFromStop's one call into the Runners port
// (ReconcilePendingPromptFromLedger) with a no-op; nothing in this test
// exercises the prompt journal.
type noopRunners struct{ Runners }

func (noopRunners) ReconcilePendingPromptFromLedger(context.Context, domain.Chat) error {
	return nil
}

// TestRegression_TurnStopWithAnOpenWaitCallKeepsChatWorking guards the live-
// reported bug: codex ends its OWN top-level turn the instant it delegates to
// a subagent — its turn_stop hook reports AsyncWork=0, because codex has no
// async-work level of its own to restate (see StopTurn's and
// fallbackAsyncWork's own doc). Nothing but Crowbar's OWN open-tool-call
// fallback (OpenWork, consulted here through fallbackAsyncWork) stands
// between that 0 and a dark spinner under a subagent that is still, visibly,
// running. Reported live, repeatedly, against this exact repo: the "wait"
// tool call sat Status==Running in the activity ledger while chat.Working
// read false and the composer showed idle.
func TestRegression_TurnStopWithAnOpenWaitCallKeepsChatWorking(t *testing.T) {
	chats := &foldingChats{}
	turns := New(Deps{
		Chats:         chats,
		Activity:      openWaitActivity{},
		Work:          inflight.NewWork(),
		InflightTurns: inflight.NewTurns(),
		Runners:       raceRunners{},
	})
	turns.SetRunners(noopRunners{})

	err := turns.closeTurnFromStop(
		t.Context(),
		domain.Chat{ID: "chat-1"},
		engineagents.Runner{ID: "runner-1", ProviderID: "codex"},
		nil,
		engineagents.CanonicalEvent{
			Kind: "turn_stop", TurnID: "t1", AsyncWork: 0,
			Message: "The implementation subagent is now running the real workspace task.",
		},
	)
	require.NoError(t, err)

	require.True(t, chats.working,
		"codex's own turn_stop reported AsyncWork=0, but a 'wait' tool call was still "+
			"Status==Running in the activity ledger — fallbackAsyncWork's OpenWork check must "+
			"have caught it and kept the chat working; it did not")
}
