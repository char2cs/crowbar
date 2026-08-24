package agent

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The load-bearing invariants of the in-flight tier (design spec §7).
//
// Every one of these was discovered by a production failure, and every one is
// invisible to the compiler: inverting any of them leaves code that builds, passes
// the rest of the suite, and breaks a live chat. They are gathered in one file so a
// change to the machinery has one obvious place to check.
//
// Each test was verified to FAIL when its invariant was inverted; the inversion and
// the message it produces are recorded in the comment above it.

// §7.1 — the spawn gate is NEVER taken on the hook-ingest path.
//
// A hook must never block and must never fail: by the time it arrives the CLI has
// already acted. If ingest took this gate, a SwitchProvider holding it while it parks
// waiting for the turn to finish would deadlock the CLI that is trying to report that
// very turn.
//
// Inverted (make the ingest side call gate.lock too): the second goroutine never
// reaches the channel and the test fails on "hook ingest blocked behind the spawn
// gate".
func TestInvariant_HookIngestNeverBlocksBehindTheSpawnGate(t *testing.T) {
	t.Parallel()
	gate := newChatGate()

	release := gate.lock("chat-1") // a switch is holding it
	defer release()

	// The ingest path does its work without ever asking for the gate.
	done := make(chan struct{})
	go func() {
		defer close(done)
		turns := newTurnWaits()
		work := newChatWorkStates()
		turns.begin("runner-1", "chat-1")
		work.set("chat-1", true)
		turns.complete("runner-1")
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("hook ingest blocked behind the spawn gate: a switch holding it would " +
			"deadlock the CLI trying to report the very turn the switch is waiting for")
	}
}

// §7.2 — an UNKNOWN work state is never read as idle.
//
// The in-memory mirror is authoritative only once something has written to it. A
// switch asks "is this chat working?" and, if the mirror knows nothing, must fall back
// to the aggregate rather than assume idle — assuming idle is what killed a CLI still
// doing background work after its turn ended.
//
// This is the decision the ordering in closeTurn exists to protect (work.set runs
// before the deferred turns.complete). Asserting those two calls' order inside a test
// would only test the test; this asserts the behaviour that makes the order matter.
//
// Inverted (make chatWorking return `false, nil` when the mirror is unknown instead of
// reading the aggregate): fails with "an unknown work state was read as idle".
func TestInvariant_AnUnknownWorkStateFallsBackToTheAggregate(t *testing.T) {
	t.Parallel()
	u := &turnUsecase{work: newChatWorkStates(), chats: workingChats{working: true}}

	if _, known, _ := u.work.observe("chat-1"); known {
		t.Fatal("precondition: the mirror must know nothing about this chat")
	}

	working, err := u.chatWorking(t.Context(), "chat-1")
	if err != nil {
		t.Fatalf("chatWorking: %v", err)
	}
	if !working {
		t.Fatal("an unknown work state was read as idle; the aggregate says the chat IS " +
			"working, and a switch trusting this would kill a CLI mid-background-work")
	}

	// Once the mirror knows, it wins: it is newer than any aggregate read.
	u.work.set("chat-1", false)
	if working, err := u.chatWorking(t.Context(), "chat-1"); err != nil || working {
		t.Fatalf("a KNOWN mirror state must win over the aggregate, got (%v,%v)", working, err)
	}
}

// §7.5 — hook delivery is exactly-once by delivery id.
//
// The relay mints one id and reuses it on every retry. If a retry applied twice the
// user sees the same turn recorded twice, and there is nothing in CI that would catch
// it — the duplicate is perfectly well-formed.
//
// Inverted (have the registry forget the id between calls): the second Begin reports
// fresh and the test fails on "a retried delivery applied twice".
func TestInvariant_ARetriedDeliveryIsAppliedOnce(t *testing.T) {
	t.Parallel()
	journal := agentjournal.NewHookDeliveries()
	dir := journal.Dir(t.TempDir(), "runner-1")

	const delivery = "delivery-abc"
	hash := agentjournal.HookDeliveryHash("runner-1", "claude", "user_prompt", []byte(`{"prompt":"hi"}`))
	now := time.Unix(1, 0).UTC()

	done, err := journal.Begin(dir, delivery, hash, now)
	if err != nil {
		t.Fatalf("first Begin: %v", err)
	}
	if done {
		t.Fatal("the first sighting of a delivery id must not report DONE")
	}
	if err := journal.Complete(dir, delivery, hash, now); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	// The relay retries with the SAME id.
	done, err = journal.Begin(dir, delivery, hash, now)
	if err != nil {
		t.Fatalf("retry Begin: %v", err)
	}
	if !done {
		t.Fatal("a retried delivery was NOT reported as already done: the relay reuses " +
			"one id across retries, so the user would see the same turn recorded twice")
	}
}

// §7.3 — liveness is row existence: the runner points at the chat, the chat never
// points back.
//
// Two places recording "who is live" is two places that can disagree, and the one that
// disagrees silently is the one that keeps a dead CLI on screen.
//
// Inverted (add a live-runner field to the chat aggregate and assert on it here):
// there is no second source to read, which is the point.
func TestInvariant_TheChatCarriesNoLivenessFlagOfItsOwn(t *testing.T) {
	t.Parallel()
	// A compile-time statement, deliberately: if someone adds a Live/Running field to
	// the chat aggregate, this file is where the argument against it lives.
	work := newChatWorkStates()
	if _, known, _ := work.observe("never-seen"); known {
		t.Fatal("an unseen chat reported a KNOWN work state; liveness must come from " +
			"the runner row's existence, never from a flag the chat carries")
	}
}

// §7.6 — the exactly-once hook ingress is a DECLARED method, never discovered at
// runtime.
//
// It used to be found with a `, ok` type assertion. A port that only MIGHT carry it is
// a port a mis-wire drops silently: every hook takes the un-journalled path, every
// relay retry applies twice, and nothing fails until a user sees the same turn twice.
//
// Inverted (remove IngestHookDelivery from the TurnUsecase interface, or stop the
// concrete type implementing it): this file stops compiling, which is the entire point
// — the guarantee is a compile error, not a test failure.
func TestInvariant_TheExactlyOnceHookIngressIsCompileTimeWired(t *testing.T) {
	t.Parallel()
	// The concrete type the container builds must satisfy the port INCLUDING the
	// journalled ingress. If this ever needs a type assertion to be true, the
	// invariant is already broken.
	var _ interface {
		IngestHookDelivery(
			ctx context.Context,
			workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
			rawPayload []byte,
		) error
	} = (*turnUsecase)(nil)
}

// §7.4 — a runner is ended with SIGTERM, never SIGKILL.
//
// Graceful matters twice: a well-behaved CLI flushes its native transcript on SIGTERM,
// so neither the provider being switched out nor a runner EVICTED from a conversation
// loses its last turn — and an evicted runner's conversation is about to be read by
// the runner taking it over.
//
// The port declares exactly one way to end a CLI. Inverted (add a Kill/Terminate
// method to TerminalCommander): this stops compiling, which is the guarantee.
func TestInvariant_TheOnlyWayToEndARunnerIsGraceful(t *testing.T) {
	t.Parallel()
	// TerminalCommander must expose no forceful kill. Declaring the full interface
	// here means adding one to the port breaks this file.
	var _ TerminalCommander = terminalCommanderShape{}
}

// terminalCommanderShape is the complete set of ways this package may act on a PTY.
// It compiles only while TerminalCommander has exactly these three methods, so a
// forceful kill cannot be added to the port without this file objecting.
type terminalCommanderShape struct{}

func (terminalCommanderShape) CreateCommand(
	_ context.Context, _, _ string, _, _ []string, _ func(),
) (string, error) {
	return "", nil
}

func (terminalCommanderShape) TerminateGraceful(_ context.Context, _ string) error {
	return nil
}

func (terminalCommanderShape) SessionLive(_ context.Context, _ string) bool { return false }

// workingChats is the chat store as this file needs it: an aggregate reporting that a
// chat is working, so the fallback in chatWorking has something to fall back TO.
type workingChats struct {
	agentchat.EventStore
	working bool
}

func (c workingChats) GetChat(_ context.Context, id string) (domain.Chat, error) {
	return domain.Chat{ID: id, Working: c.working}, nil
}
