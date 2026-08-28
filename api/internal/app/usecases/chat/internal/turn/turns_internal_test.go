package turn

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
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
		r.messages.Observe("c", "t", "m", i, true, false, "increment", time.Now())
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

// fakeChoiceActivity is a minimal agentactivity.EventStore recording only the
// three calls the permission/auto-approve path makes — Interrupt, OpenChoice,
// AnswerChoice — mirroring the embed-and-override shape harness_test.go's own
// faultWriteActivity uses one package over, but scoped to this package's own
// narrower needs rather than importing that internal test type across
// packages.
type fakeChoiceActivity struct {
	agentactivity.EventStore
	mu       sync.Mutex
	answered []answeredChoice
}

type answeredChoice struct {
	chatID, choiceID string
	optionIDs        []string
	auto             bool
}

func (f *fakeChoiceActivity) Interrupt(
	context.Context, string, string, string, string, time.Time,
) error {
	return nil
}

func (f *fakeChoiceActivity) OpenChoice(context.Context, agentactivity.ChoiceInput) error {
	return nil
}

func (f *fakeChoiceActivity) AnswerChoice(
	_ context.Context, chatID, choiceID string, optionIDs []string, auto bool, _ time.Time,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = append(f.answered, answeredChoice{
		chatID: chatID, choiceID: choiceID, optionIDs: optionIDs, auto: auto,
	})
	return nil
}

// newChoiceTestTurns builds a real *Turns over the real claude/codex agent
// registry (never a hand-rolled Agent fake — see this task's own note above),
// a real in-memory answerdesk.Desk, and the two narrow fakes this file
// already has a pattern for. Every field New(Deps{}) doesn't set here is left
// zero-value deliberately: the permission-event path never touches Telemetry,
// Workspace, or the inflight gates, so a nil there must never be reached — if
// a future change makes it reach one, that is this test correctly catching a
// widened blast radius, not a fixture gap to paper over.
func newChoiceTestTurns(
	t *testing.T,
) (*Turns, *fakeChoiceActivity) {
	t.Helper()
	activity := &fakeChoiceActivity{}
	turns := New(Deps{
		Chats:            raceChats{},
		Activity:         activity,
		Agents:           agents.New(),
		Answers:          answerdesk.New(answerdesk.DefaultRetention, nil),
		PermissionLevels: permission.New(),
		Home:             func() (string, error) { return t.TempDir(), nil },
	})
	return turns, activity
}

// permissionChoiceEvent sets PromptID deliberately: choiceID() (observation.go)
// falls back to an unpredictable, timestamp-based id whenever PromptID is
// empty, which would make a test's expected id nondeterministic. Every test
// below computes its expected id by calling the real choiceID() function
// rather than hand-duplicating its concatenation rule, so a future change to
// that rule cannot silently desync a hand-written literal from the code
// actually running.
func permissionChoiceEvent(
	toolName string,
	risk agents.RiskTier,
) agents.CanonicalEvent {
	return agents.CanonicalEvent{
		Kind: agents.HookPermission,
		Choice: &agents.ChoicePrompt{
			Kind:     agents.ChoiceToolPermission,
			PromptID: "p1",
			ToolName: toolName,
			Risk:     risk,
			Options: []agents.ChoiceOption{
				{ID: domain.ChoiceOptionAllow, Kind: domain.ChoiceOptionAllow, Label: "Allow"},
				{ID: domain.ChoiceOptionDeny, Kind: domain.ChoiceOptionDeny, Label: "Deny"},
			},
		},
		Interrupt: &agents.InterruptEvent{Kind: agents.InterruptPermission},
	}
}

func TestOpenChoice_TrustedLevelAutoApprovesAStandardTierPromptWithNoHumanHold(t *testing.T) {
	turns, activity := newChoiceTestTurns(t)
	turns.permissionLevels.Set("chat-1", permission.Trusted)
	ctx := inflight.WithDeliveryID(context.Background(), "delivery-1")
	agent, err := turns.agents.Get(ctx, t.TempDir(), "claude")
	require.NoError(t, err)
	runner := agents.Runner{ID: "r1", ProviderID: "claude", CurrentChatID: "chat-1"}
	ev := permissionChoiceEvent("Bash", agents.RiskStandard)

	err = turns.handleObservation(ctx, runner, agent, ev, []byte(`{}`))

	require.NoError(t, err)
	id := choiceID("chat-1", ev.Choice)
	_, held := turns.answers.ByChoiceID(id)
	assert.False(t, held, "a trusted-level standard-tier prompt must auto-resolve, not wait for a human")
	require.Len(t, activity.answered, 1)
	assert.Equal(t, []string{domain.ChoiceOptionAllow}, activity.answered[0].optionIDs,
		"the recorded decision must be indistinguishable from a human's own Allow click")
	assert.True(t, activity.answered[0].auto,
		"the ledger write must be tagged as policy-approved, not a human's own click")
}

func TestOpenChoice_GuardedLevelHoldsASensitiveTierPromptForAHuman(t *testing.T) {
	turns, activity := newChoiceTestTurns(t)
	// No Set call: the chat is unseen, so permission.Store.Get defaults to Guarded.
	ctx := inflight.WithDeliveryID(context.Background(), "delivery-2")
	agent, err := turns.agents.Get(ctx, t.TempDir(), "claude")
	require.NoError(t, err)
	runner := agents.Runner{ID: "r1", ProviderID: "claude", CurrentChatID: "chat-2"}
	ev := permissionChoiceEvent("WebFetch", agents.RiskSensitive)

	err = turns.handleObservation(ctx, runner, agent, ev, []byte(`{}`))

	require.NoError(t, err)
	id := choiceID("chat-2", ev.Choice)
	_, held := turns.answers.ByChoiceID(id)
	assert.True(t, held, "guarded must still hold a sensitive-tier prompt for a human, unchanged")
	assert.Empty(t, activity.answered, "nothing must be auto-decided while the prompt is held")
}

// TestRegression_CrowbarsOwnMCPToolCallNeverHoldsForAHumanOnAnyProvider is the
// parity fix: Crowbar's own injected tool calls run in a pane with no human
// present to answer a modal (see codex.yaml's default_tools_approval_mode
// comment) — RiskInternal must bypass the level dial entirely, even at the
// strictest level, on EITHER provider, since Risk is a canonical field the
// engine reasons about directly rather than a per-provider CLI workaround.
func TestRegression_CrowbarsOwnMCPToolCallNeverHoldsForAHumanOnAnyProvider(t *testing.T) {
	for _, providerID := range []string{"claude", "codex"} {
		t.Run(providerID, func(t *testing.T) {
			turns, activity := newChoiceTestTurns(t)
			// Guarded, the strictest level — proving RiskInternal bypasses the dial
			// entirely rather than merely being permitted at a lenient one.
			ctx := inflight.WithDeliveryID(context.Background(), "delivery-"+providerID)
			agent, err := turns.agents.Get(ctx, t.TempDir(), providerID)
			require.NoError(t, err)
			chatID := "chat-" + providerID
			runner := agents.Runner{ID: "r1", ProviderID: providerID, CurrentChatID: chatID}
			ev := permissionChoiceEvent("mcp__crowbar__resolve_review_thread", agents.RiskInternal)

			err = turns.handleObservation(ctx, runner, agent, ev, []byte(`{}`))

			require.NoError(t, err)
			id := choiceID(chatID, ev.Choice)
			_, held := turns.answers.ByChoiceID(id)
			assert.False(t, held, "an internal-tier call must never stall waiting for a human")
			require.Len(t, activity.answered, 1,
				"the prompt must be auto-decided through the real render-and-resolve path, not merely never held")
		})
	}
}
