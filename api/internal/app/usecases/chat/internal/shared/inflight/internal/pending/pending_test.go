package pending_test

import (
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight/internal/pending"
)

func drain(p *pending.Hooks, runnerID string) ([]pending.Hook, bool) {
	var got []pending.Hook
	exited := p.Finish(runnerID, func(h pending.Hook) { got = append(got, h) })
	return got, exited
}

// Outside the startup window there is nothing to buffer: handled=false is how the
// caller learns to ingest the hook directly instead of dropping it.
func TestHooks_UnregisteredRunnerIsNotHandled(t *testing.T) {
	t.Parallel()

	p := pending.New()

	handled, err := p.Enqueue("runner-1", "claude", "turn_start", []byte(`{}`))
	if handled || err != nil {
		t.Fatalf("Enqueue on an unregistered runner = (handled=%v, err=%v), want (false, nil)", handled, err)
	}
}

func TestHooks_ReplaysInArrivalOrder(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for _, event := range []string{"session_start", "user_prompt", "turn_start"} {
		handled, err := p.Enqueue("runner-1", "claude", event, []byte(event))
		if !handled || err != nil {
			t.Fatalf("Enqueue(%s) = (handled=%v, err=%v), want (true, nil)", event, handled, err)
		}
	}

	got, exited := drain(p, "runner-1")
	if exited {
		t.Fatal("Finish reported the CLI exited during startup when it had not")
	}
	if len(got) != 3 {
		t.Fatalf("replayed %d hooks, want 3", len(got))
	}
	for i, want := range []string{"session_start", "user_prompt", "turn_start"} {
		if got[i].CanonicalEvent != want {
			t.Fatalf("hook %d = %q, want %q: replay is out of order", i, got[i].CanonicalEvent, want)
		}
		if string(got[i].RawPayload) != want {
			t.Fatalf("hook %d payload = %q, want %q", i, got[i].RawPayload, want)
		}
		if got[i].Provider != "claude" {
			t.Fatalf("hook %d provider = %q, want claude", i, got[i].Provider)
		}
	}
}

// The relay retries. Two arrivals of one delivery id inside the window must be
// buffered once, or the exactly-once ingress boundary is breached on replay.
func TestHooks_DeliveryIDIsDeduplicated(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	for range 3 {
		handled, err := p.EnqueueDelivery(
			"runner-1", "claude", "turn_start", []byte(`{}`), "delivery-1", "/tmp/d", "hash-1",
		)
		if !handled || err != nil {
			t.Fatalf("EnqueueDelivery = (handled=%v, err=%v), want (true, nil)", handled, err)
		}
	}

	got, _ := drain(p, "runner-1")
	if len(got) != 1 {
		t.Fatalf("replayed %d hooks for one delivery id, want 1", len(got))
	}
	if got[0].DeliveryID != "delivery-1" || got[0].DeliveryDir != "/tmp/d" || got[0].DeliveryHash != "hash-1" {
		t.Fatalf("delivery fields lost in the buffer: %+v", got[0])
	}
}

// Two DIFFERENT deliveries are two hooks; the dedupe must key on the id, not on
// "has a delivery id at all".
func TestHooks_DistinctDeliveriesAreBothBuffered(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	for _, id := range []string{"d-1", "d-2"} {
		if _, err := p.EnqueueDelivery("runner-1", "claude", "turn_start", []byte(`{}`), id, "", ""); err != nil {
			t.Fatalf("EnqueueDelivery(%s): %v", id, err)
		}
	}

	if got, _ := drain(p, "runner-1"); len(got) != 2 {
		t.Fatalf("replayed %d hooks for two delivery ids, want 2", len(got))
	}
}

// The buffer is bounded. A CLI that floods its own startup window must be told
// the buffer is full — still handled, because ingesting it directly would race
// the runner row it is waiting for.
func TestHooks_CountLimitIsEnforcedAndStillHandled(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	var lastErr error
	overflowed := 0
	for range 70 {
		handled, err := p.Enqueue("runner-1", "claude", "turn_start", []byte(`{}`))
		if !handled {
			t.Fatal("an over-limit hook reported handled=false; it would be ingested before its runner row exists")
		}
		if err != nil {
			overflowed++
			lastErr = err
		}
	}
	if overflowed == 0 {
		t.Fatal("70 hooks fitted in a 64-hook buffer; the limit is not enforced")
	}
	if lastErr == nil || !strings.Contains(lastErr.Error(), "startup buffer limit exceeded") {
		t.Fatalf("overflow error = %v, want a startup buffer limit error", lastErr)
	}
	if got, _ := drain(p, "runner-1"); len(got) != 64 {
		t.Fatalf("buffered %d hooks, want the 64 that fitted", len(got))
	}
}

func TestHooks_ByteLimitIsEnforced(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	huge := make([]byte, 24<<20)
	if _, err := p.Enqueue("runner-1", "claude", "turn_start", huge); err != nil {
		t.Fatalf("the first 24MB hook was rejected: %v", err)
	}
	if _, err := p.Enqueue("runner-1", "claude", "turn_start", huge); err == nil {
		t.Fatal("48MB of payload fitted in a 32MB buffer; the byte limit is not enforced")
	}
}

// The buffer owns its bytes. A caller that reuses its read buffer must not be
// able to rewrite a hook that is already queued.
func TestHooks_PayloadIsCopied(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	payload := []byte("original")
	if _, err := p.Enqueue("runner-1", "claude", "turn_start", payload); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	copy(payload, "OVERWRIT")

	got, _ := drain(p, "runner-1")
	if len(got) != 1 || string(got[0].RawPayload) != "original" {
		t.Fatalf("replayed payload = %q, want %q: the buffer aliased the caller's slice", got[0].RawPayload, "original")
	}
}

// Two spawns claiming one runner id is a bug, not a race to absorb: the second
// Register must fail rather than silently reset the first one's buffer.
func TestHooks_DoubleRegisterFails(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := p.Register("runner-1"); err == nil {
		t.Fatal("registering one runner id twice succeeded")
	}
}

func TestHooks_DiscardDropsTheWindow(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := p.Enqueue("runner-1", "claude", "turn_start", []byte(`{}`)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	p.Discard("runner-1")

	got, exited := drain(p, "runner-1")
	if len(got) != 0 || exited {
		t.Fatalf("after Discard, Finish replayed %d hooks (exited=%v); the spawn failed and the row will never exist", len(got), exited)
	}
	if handled, _ := p.Enqueue("runner-1", "claude", "turn_start", []byte(`{}`)); handled {
		t.Fatal("a discarded runner still absorbed hooks")
	}
}

// A CLI can die inside its own startup window. Finish is how the spawn learns
// that, since no exit hook could reach a runner row that did not exist yet.
func TestHooks_MarkExitedIsReportedByFinish(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if !p.MarkExited("runner-1") {
		t.Fatal("MarkExited reported the runner was not registered")
	}
	if _, exited := drain(p, "runner-1"); !exited {
		t.Fatal("Finish did not report the startup-window exit")
	}
	if p.MarkExited("runner-2") {
		t.Fatal("MarkExited claimed an unregistered runner")
	}
}

// Finish keeps draining until a pass finds the buffer empty. That loop is what
// makes the handoff from buffered to direct ingest atomic: a hook that arrives
// WHILE the replay runs is replayed too, never dropped between the two regimes.
func TestHooks_FinishDrainsHooksEnqueuedDuringReplay(t *testing.T) {
	t.Parallel()

	p := pending.New()
	if err := p.Register("runner-1"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := p.Enqueue("runner-1", "claude", "first", []byte(`{}`)); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	var seen []string
	p.Finish("runner-1", func(h pending.Hook) {
		seen = append(seen, h.CanonicalEvent)
		if h.CanonicalEvent == "first" {
			if handled, err := p.Enqueue("runner-1", "claude", "second", []byte(`{}`)); !handled || err != nil {
				t.Errorf("re-entrant Enqueue = (handled=%v, err=%v), want (true, nil)", handled, err)
			}
		}
	})

	if len(seen) != 2 || seen[0] != "first" || seen[1] != "second" {
		t.Fatalf("replayed %v, want [first second]: a hook arriving during replay was dropped", seen)
	}
	if handled, _ := p.Enqueue("runner-1", "claude", "third", []byte(`{}`)); handled {
		t.Fatal("the window stayed open after Finish drained it")
	}
}

func TestHooks_RunnersAreIndependent(t *testing.T) {
	t.Parallel()

	p := pending.New()
	for _, id := range []string{"runner-1", "runner-2"} {
		if err := p.Register(id); err != nil {
			t.Fatalf("Register(%s): %v", id, err)
		}
		if _, err := p.Enqueue(id, "claude", id, []byte(`{}`)); err != nil {
			t.Fatalf("Enqueue(%s): %v", id, err)
		}
	}

	got, _ := drain(p, "runner-1")
	if len(got) != 1 || got[0].CanonicalEvent != "runner-1" {
		t.Fatalf("runner-1 replayed %+v, want its own single hook", got)
	}
	got, _ = drain(p, "runner-2")
	if len(got) != 1 || got[0].CanonicalEvent != "runner-2" {
		t.Fatalf("runner-2 lost its buffer when runner-1 finished: %+v", got)
	}
}
