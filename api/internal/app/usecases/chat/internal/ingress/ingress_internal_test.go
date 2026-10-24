package ingress

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type fakeRunners struct {
	mu   sync.Mutex
	byID map[string]engineagents.Runner
	// recorded are the runners only their chats' ledgers still know.
	recorded map[string]engineagents.Runner
}

func (r *fakeRunners) RecordedRunner(_ context.Context, id string) (engineagents.Runner, bool, error) {
	runner, ok := r.recorded[id]
	return runner, ok, nil
}

func (r *fakeRunners) RunnerByID(id string) (engineagents.Runner, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	runner, ok := r.byID[id]
	return runner, ok
}

func (r *fakeRunners) remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
}

// fakeAgent is a descriptor whose parse answers what the test set.
type fakeAgent struct {
	engineagents.Agent
	parsed engineagents.CanonicalEvent
	err    error
}

func (a fakeAgent) ParseHook(string, []byte, engineagents.Channel) (engineagents.CanonicalEvent, error) {
	return a.parsed, a.err
}

type fakeDescriptors struct {
	agent fakeAgent
	err   error
	// resolved, when set, is told each time a hook's runner has been resolved.
	resolved chan struct{}
}

func (d *fakeDescriptors) Descriptor(context.Context, engineagents.Runner) (engineagents.Agent, error) {
	if d.resolved != nil {
		d.resolved <- struct{}{}
	}
	return d.agent, d.err
}

type dispatched struct {
	runner  engineagents.Runner
	ev      engineagents.CanonicalEvent
	channel engineagents.Channel
}

type fakeTurns struct {
	dispatched []dispatched
	marked     []ledger.Row
	telemetry  int
	reply      []byte
	ask        string
	err        error
	// during runs inside Dispatch, while the chat's lock is held.
	during func()
}

func (t *fakeTurns) RecordTelemetry(context.Context, engineagents.Runner, engineagents.Agent, []byte) error {
	t.telemetry++
	return t.err
}

func (t *fakeTurns) Mark(_ context.Context, _ engineagents.Runner, row ledger.Row) error {
	if t.err != nil {
		return t.err
	}
	t.marked = append(t.marked, row)
	return nil
}

func (t *fakeTurns) Dispatch(
	_ context.Context, runner engineagents.Runner, _ engineagents.Agent,
	ev engineagents.CanonicalEvent, _ []byte, channel engineagents.Channel,
) ([]byte, string, error) {
	t.dispatched = append(t.dispatched, dispatched{runner: runner, ev: ev, channel: channel})
	if t.during != nil {
		t.during()
	}
	return t.reply, t.ask, t.err
}

type fakeLedger struct {
	letters     []ledgerstore.DeadLetter
	failLetters error
}

func (*fakeLedger) DeadLetterCount(context.Context, ...string) (int, error) { return 0, nil }

func (l *fakeLedger) DeadLetter(_ context.Context, d ledgerstore.DeadLetter) error {
	if l.failLetters != nil {
		return l.failLetters
	}
	l.letters = append(l.letters, d)
	return nil
}

type fixture struct {
	runners     *fakeRunners
	descriptors *fakeDescriptors
	turns       *fakeTurns
	ledger      *fakeLedger
	locks       *Locks
	ingress     *Ingress
}

func newFixture() fixture {
	f := fixture{
		runners: &fakeRunners{byID: map[string]engineagents.Runner{
			"runner-1": {ID: "runner-1", ProviderID: "p", CurrentChatID: "chat-1"},
		}},
		descriptors: &fakeDescriptors{},
		turns:       &fakeTurns{},
		ledger:      &fakeLedger{},
		locks:       NewLocks(),
	}
	f.ingress = New(Deps{
		Runners: f.runners, Descriptors: f.descriptors, Turns: f.turns, Ledger: f.ledger, Locks: f.locks,
	})
	return f
}

// hold takes chat-1's lock as a user-initiated path would.
func (f fixture) hold(t *testing.T) func() {
	t.Helper()
	unlock, err := f.locks.Lock(t.Context(), "chat-1")
	require.NoError(t, err)
	return unlock
}

func (f fixture) reasons() []string {
	out := []string{}
	for _, l := range f.ledger.letters {
		out = append(out, l.Reason)
	}
	return out
}

func TestHook_UnknownRunnerIsNotFoundAndDeadLettered(t *testing.T) {
	f := newFixture()

	_, err := f.ingress.Hook(t.Context(), "runner-gone", "p", "tool_pre", []byte(`{}`), engineagents.ChannelHooks)

	require.ErrorIs(t, err, ErrUnknownRunner)
	require.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Equal(t, []string{reasonUnknownRunner}, f.reasons())
	assert.Equal(t, "runner-gone", f.ledger.letters[0].RunnerID)
	assert.Empty(t, f.turns.dispatched)
}

func TestHook_UnmappablePayloadRecordsAMarkerAndIsRefused(t *testing.T) {
	f := newFixture()
	f.descriptors.agent.err = fmt.Errorf("message: %w", engineagents.ErrRequiredFieldMissing)

	_, err := f.ingress.Hook(t.Context(), "runner-1", "p", "user_prompt", []byte(`{}`), engineagents.ChannelHooks)

	require.ErrorIs(t, err, ErrUnmappable)
	require.ErrorIs(t, err, apperr.ErrUnprocessable)
	require.Len(t, f.turns.marked, 1, "one marker on the runner's chat")
	marker, decodeErr := ledger.DecodeBody[ledger.Marker](f.turns.marked[0])
	require.NoError(t, decodeErr)
	assert.Equal(t, ledger.MarkerDeadLetter, marker.Kind)
	assert.Equal(t, "user_prompt: "+f.descriptors.agent.err.Error(), marker.Detail)
	assert.Equal(t, []string{reasonUnmappable}, f.reasons())
	assert.Empty(t, f.turns.dispatched)
}

func TestHook_ForeignConversationIsAcknowledgedAndDeadLettered(t *testing.T) {
	for name, foreign := range map[string]func(f fixture){
		"refused by the descriptor": func(f fixture) {
			f.descriptors.agent.err = engineagents.ErrForeignConversation
		},
		"another session of the chat's CLI": func(f fixture) {
			f.turns.err = fmt.Errorf("names another session: %w", engineagents.ErrForeignConversation)
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			foreign(f)

			reply, err := f.ingress.Hook(t.Context(), "runner-1", "p", "tool_pre", []byte(`{}`), engineagents.ChannelHooks)

			require.NoError(t, err, "the CLI's own internal work is not an error to it")
			assert.Equal(t, Reply{}, reply)
			assert.Equal(t, []string{reasonForeign}, f.reasons())
			assert.Equal(t, "chat-1", f.ledger.letters[0].ChatID)
		})
	}
}

func TestHook_AnAskIsHeldOnItsAskID(t *testing.T) {
	f := newFixture()
	f.descriptors.agent.parsed = engineagents.CanonicalEvent{Kind: engineagents.HookPermission}
	f.turns.ask = "choice-1"

	reply, err := f.ingress.Hook(t.Context(), "runner-1", "p", "permission", []byte(`{}`), engineagents.ChannelAPI)

	require.NoError(t, err)
	assert.Equal(t, Reply{Ask: "choice-1"}, reply)
	require.Len(t, f.turns.dispatched, 1)
	assert.Equal(t, engineagents.ChannelAPI, f.turns.dispatched[0].channel)
	f.hold(t)() // the hold is the sender's, never under the chat's lock
}

func TestHook_DispatchRunsUnderTheChatsLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture()
		f.turns.reply = []byte("steer")
		acquired := make(chan struct{})
		heldWhileDispatching := false
		f.turns.during = func() {
			go func() {
				if unlock, err := f.locks.Lock(context.Background(), "chat-1"); err == nil {
					unlock()
				}
				close(acquired)
			}()
			synctest.Wait()
			heldWhileDispatching = !isClosed(acquired)
		}

		reply, err := f.ingress.Hook(t.Context(), "runner-1", "p", "turn_stop", []byte(`{}`), engineagents.ChannelHooks)

		require.NoError(t, err)
		assert.True(t, heldWhileDispatching, "nothing else holds the chat while a hook records")
		<-acquired
		assert.Equal(t, Reply{Body: []byte("steer")}, reply)
		assert.Empty(t, f.ledger.letters, "a placed hook is no dead letter")
	})
}

// A sender that gives up while the chat is held is not recorded later: its
// hook is dead-lettered.
func TestHook_ASenderThatGivesUpWaitingForTheChatIsDeadLettered(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFixture()
		unlock := f.hold(t)
		defer unlock()
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := f.ingress.Hook(ctx, "runner-1", "p", "tool_pre", []byte(`{}`), engineagents.ChannelHooks)
			done <- err
		}()
		synctest.Wait()

		cancel()

		require.ErrorIs(t, <-done, context.Canceled)
		assert.Empty(t, f.turns.dispatched)
		assert.Equal(t, []string{reasonSenderGone}, f.reasons())
	})
}

// A runner that exited while its hook waited for the chat still takes it: the
// hook is its, on its chat.
func TestHook_ARunnerThatExitedWhileItsHookWaitedStillTakesIt(t *testing.T) {
	f := newFixture()
	f.descriptors.resolved = make(chan struct{})
	unlock := f.hold(t)
	done := make(chan error, 1)
	go func() {
		_, err := f.ingress.Hook(t.Context(), "runner-1", "p", "turn_stop", []byte(`{}`), engineagents.ChannelHooks)
		done <- err
	}()
	<-f.descriptors.resolved
	f.runners.remove("runner-1") // its exit takes it off the table under the chat's lock
	unlock()

	require.NoError(t, <-done)
	require.Len(t, f.turns.dispatched, 1)
	assert.Equal(t, "chat-1", f.turns.dispatched[0].runner.CurrentChatID)
	assert.Empty(t, f.ledger.letters)
}

func TestHook_AnExitedRunnerItsChatsLedgerStartedTakesItsHooks(t *testing.T) {
	f := newFixture()
	f.runners.recorded = map[string]engineagents.Runner{"runner-0": {ID: "runner-0", CurrentChatID: "chat-1"}}

	_, err := f.ingress.Hook(t.Context(), "runner-0", "p", "turn_stop", []byte(`{}`), engineagents.ChannelHooks)

	require.NoError(t, err)
	require.Len(t, f.turns.dispatched, 1)
	assert.Equal(t, "runner-0", f.turns.dispatched[0].runner.ID)
	assert.Empty(t, f.ledger.letters)
}

func TestHook_DeclaredFiltersAreAcknowledgedWithNoRecord(t *testing.T) {
	f := newFixture()
	f.descriptors.agent.err = fmt.Errorf("when: %w", engineagents.ErrHookVariantMismatch)

	reply, err := f.ingress.Hook(t.Context(), "runner-1", "p", "idle", []byte(`{}`), engineagents.ChannelHooks)

	require.NoError(t, err)
	assert.Equal(t, Reply{}, reply)
	assert.Empty(t, f.ledger.letters)
	assert.Empty(t, f.turns.dispatched)
}

func TestHook_AHookThatCouldNotBeRecordedIsRefusedAndDeadLettered(t *testing.T) {
	for name, tc := range map[string]struct {
		set    func(f fixture)
		reason string
		want   error
	}{
		"its descriptor cannot be resolved": {
			set:    func(f fixture) { f.descriptors.err = errors.New("no descriptor") },
			reason: reasonNoDescriptor,
		},
		"its chat is gone": {
			set:    func(f fixture) { f.turns.err = fmt.Errorf("chat: %w", apperr.ErrNotFound) },
			reason: reasonUnknownChat, want: apperr.ErrNotFound,
		},
		"its rows did not commit": {
			set:    func(f fixture) { f.turns.err = errors.New("ledger write failed") },
			reason: reasonWriteFailed,
		},
		"its telemetry cannot be parsed": {
			set:    func(f fixture) { f.turns.err = fmt.Errorf("telemetry: %w", apperr.ErrUnprocessable) },
			reason: reasonUnmappable, want: ErrUnmappable,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			tc.set(f)

			_, err := f.ingress.Hook(t.Context(), "runner-1", "p", "tool_pre", []byte(`{}`), engineagents.ChannelHooks)

			require.Error(t, err)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			}
			assert.Equal(t, []string{tc.reason}, f.reasons())
		})
	}
}

func TestHook_TelemetryIsRecordedUnderTheChatsLockWithoutParsing(t *testing.T) {
	f := newFixture()
	f.descriptors.agent.err = errors.New("telemetry is never parsed as a hook")

	reply, err := f.ingress.Hook(t.Context(), "runner-1", "p", engineagents.HookTelemetry, []byte(`{}`), engineagents.ChannelHooks)

	require.NoError(t, err)
	assert.Equal(t, Reply{}, reply)
	assert.Equal(t, 1, f.turns.telemetry)
	assert.Empty(t, f.ledger.letters)
}

// A hook acknowledged without rows is answered as placed only once its dead
// letter is kept.
func TestHook_AnAcknowledgedHookIsDeadLetteredOrRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		set    func(f fixture)
		reason string
	}{
		"foreign to the chat": {
			set:    func(f fixture) { f.descriptors.agent.err = engineagents.ErrForeignConversation },
			reason: reasonForeign,
		},
		"recorded nowhere by design": {
			set:    func(f fixture) { f.turns.err = fmt.Errorf("unhandled: %w", seam.ErrHookNotRecorded) },
			reason: reasonNotRecorded,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture()
			tc.set(f)
			_, err := f.ingress.Hook(t.Context(), "runner-1", "p", "user_prompt", []byte(`{}`), engineagents.ChannelHooks)
			require.NoError(t, err)
			assert.Equal(t, []string{tc.reason}, f.reasons())

			f.ledger.failLetters = errors.New("disk full")
			_, err = f.ingress.Hook(t.Context(), "runner-1", "p", "user_prompt", []byte(`{}`), engineagents.ChannelHooks)
			require.Error(t, err, "nothing kept, so nothing is acknowledged")
		})
	}
}

// An unmappable hook of a runner whose chat was purged marks nothing.
func TestHook_AnUnmappableHookOfAChatGoneMarksNothing(t *testing.T) {
	f := newFixture()
	f.descriptors.agent.err = fmt.Errorf("message: %w", engineagents.ErrRequiredFieldMissing)
	f.turns.err = fmt.Errorf("chat: %w", apperr.ErrNotFound)

	_, err := f.ingress.Hook(t.Context(), "runner-1", "p", "user_prompt", []byte(`{}`), engineagents.ChannelHooks)

	require.ErrorIs(t, err, apperr.ErrNotFound)
	assert.Empty(t, f.turns.marked, "a purged chat gets no marker")
	assert.Equal(t, []string{reasonUnknownChat}, f.reasons())
}
