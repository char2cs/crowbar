package turnstate_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight/internal/turnstate"
)

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestTurns_BeginOpensCompleteCloses(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()
	if open := w.Inflight("chat-1"); len(open) != 0 {
		t.Fatalf("a fresh registry reported %d open turns on a chat nothing has prompted", len(open))
	}

	w.Begin("runner-1", "chat-1")
	open := w.Inflight("chat-1")
	if len(open) != 1 {
		t.Fatalf("Inflight = %d turns after Begin, want 1", len(open))
	}
	if closed(open[0]) {
		t.Fatal("the turn's release channel was already closed while the turn was open")
	}

	w.Complete("runner-1")
	if !closed(open[0]) {
		t.Fatal("Complete did not close the release channel a switch is parked on")
	}
	if open := w.Inflight("chat-1"); len(open) != 0 {
		t.Fatalf("Inflight = %d turns after Complete, want 0", len(open))
	}
}

// A second prompt while the CLI is still working is the SAME turn: re-opening it
// would hand a new waiter a fresh channel and orphan the one already blocked.
func TestTurns_SecondBeginOnSameChatKeepsOneTurn(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()
	w.Begin("runner-1", "chat-1")
	first := w.Inflight("chat-1")
	w.Begin("runner-1", "chat-1")

	again := w.Inflight("chat-1")
	if len(again) != 1 {
		t.Fatalf("Inflight = %d turns after a repeat Begin, want 1", len(again))
	}
	if closed(first[0]) {
		t.Fatal("a repeat Begin closed the open turn's channel; the waiter would resume mid-answer")
	}

	w.Complete("runner-1")
	if !closed(first[0]) {
		t.Fatal("the ORIGINAL channel stayed open after Complete; a repeat Begin had replaced it")
	}
}

// The runner moved without us being told. Its old turn can never be closed where
// it stands, so Begin releases it rather than leaking a waiter.
func TestTurns_BeginOnAnotherChatReleasesTheOldTurn(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()
	w.Begin("runner-1", "chat-1")
	old := w.Inflight("chat-1")

	w.Begin("runner-1", "chat-2")

	if !closed(old[0]) {
		t.Fatal("moving a runner to another chat left the old chat's waiter parked forever")
	}
	if open := w.Inflight("chat-1"); len(open) != 0 {
		t.Fatalf("chat-1 still reports %d open turns after its runner moved away", len(open))
	}
	if open := w.Inflight("chat-2"); len(open) != 1 {
		t.Fatalf("chat-2 reports %d open turns after the runner moved onto it, want 1", len(open))
	}
}

// Every path that can end a turn calls Complete unconditionally, so completing a
// runner with no open turn must be silent rather than a panic or a stray signal.
func TestTurns_CompleteWithoutBeginIsSilent(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()
	_, changed := w.Watch("chat-1")

	w.Complete("runner-nobody")

	if closed(changed) {
		t.Fatal("completing an unknown runner signalled a chat transition that never happened")
	}
}

func TestTurns_EmptyIdentifiersAreIgnored(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()
	w.Begin("", "chat-1")
	w.Begin("runner-1", "")
	w.Complete("")

	if open := w.Inflight("chat-1"); len(open) != 0 {
		t.Fatalf("an empty runner id opened %d turns", len(open))
	}
}

func TestTurns_WatchSignalsTheNextTransition(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()

	open, changed := w.Watch("chat-1")
	if open {
		t.Fatal("Watch reported an open turn on an idle chat")
	}

	w.Begin("runner-1", "chat-1")
	if !closed(changed) {
		t.Fatal("Watch's signal did not close on Begin; a switch would sleep through the turn starting")
	}

	open, changed = w.Watch("chat-1")
	if !open {
		t.Fatal("Watch reported no open turn while one was in flight")
	}

	w.Complete("runner-1")
	if !closed(changed) {
		t.Fatal("Watch's signal did not close on Complete; a switch would sleep through the turn ending")
	}
}

// Inflight returns EVERY open turn on the chat. One waiter that watched only the
// first would sail past the second, which the hook-path placement healers can
// create.
func TestTurns_InflightReturnsEveryOpenTurnOnTheChat(t *testing.T) {
	t.Parallel()

	w := turnstate.NewTurns()
	w.Begin("runner-1", "chat-1")
	w.Begin("runner-2", "chat-1")

	if open := w.Inflight("chat-1"); len(open) != 2 {
		t.Fatalf("Inflight = %d turns, want 2: a second runner on the chat was invisible", len(open))
	}
}

func TestWork_UnknownUntilACommandSaysOtherwise(t *testing.T) {
	t.Parallel()

	w := turnstate.NewWork()

	working, known, changed := w.Observe("chat-1")
	if known || working {
		t.Fatalf("a chat no command has touched reported known=%v working=%v, want false/false", known, working)
	}
	if closed(changed) {
		t.Fatal("the change signal of an untouched chat was already closed")
	}

	w.Set("chat-1", true)
	if !closed(changed) {
		t.Fatal("Set did not close the signal a waiter had already taken")
	}

	working, known, _ = w.Observe("chat-1")
	if !known || !working {
		t.Fatalf("after Set(true) got known=%v working=%v, want true/true", known, working)
	}
}

// A Stop can restate a lower async-work level while Working stays true. The
// restatement is still newer, so it must wake the waiters.
func TestWork_SignalsEvenWhenTheFlagDidNotChange(t *testing.T) {
	t.Parallel()

	w := turnstate.NewWork()
	w.Set("chat-1", true)

	_, _, changed := w.Observe("chat-1")
	w.Set("chat-1", true)

	if !closed(changed) {
		t.Fatal("a restated Working=true did not wake waiters")
	}
}

func TestWork_EmptyChatIDIsIgnored(t *testing.T) {
	t.Parallel()

	w := turnstate.NewWork()
	w.Set("", true)

	if _, known, _ := w.Observe(""); known {
		t.Fatal("Set with an empty chat id recorded a state")
	}
}

func TestWork_ChatsAreIndependent(t *testing.T) {
	t.Parallel()

	w := turnstate.NewWork()
	w.Set("chat-1", true)

	if _, known, _ := w.Observe("chat-2"); known {
		t.Fatal("setting one chat's work state made another chat known")
	}
}
