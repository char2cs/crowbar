package gate_test

import (
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight/internal/gate"
)

// The gate's whole job is that two spawns on ONE chat never overlap. Proving it
// with timing would prove nothing, so the section it protects is a deliberately
// unsynchronised counter: a gate that does not serialise is a data race, and the
// suite runs under -race.
func TestGate_SerialisesOneChat(t *testing.T) {
	t.Parallel()

	g := gate.New()
	const callers = 64

	counter := 0
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release := g.Lock("chat-1")
			defer release()
			counter++
		}()
	}
	close(start)
	wg.Wait()

	if counter != callers {
		t.Fatalf("counter = %d, want %d: the gate let two callers into one chat's section", counter, callers)
	}
}

// Two chats are two gates. Taking both from ONE goroutine deadlocks instantly if
// the implementation ever collapses to a single process-wide lock, which is the
// regression this pins: a global lock would serialise every chat in the daemon
// behind the slowest spawn.
func TestGate_DifferentChatsDoNotBlockEachOther(t *testing.T) {
	t.Parallel()

	g := gate.New()

	releaseA := g.Lock("chat-a")
	releaseB := g.Lock("chat-b")
	releaseB()
	releaseA()
}

// A released gate is reusable. The release func drops the chat's entry once the
// last holder leaves, so the second Lock exercises the re-create path rather than
// a leftover entry — and a release that failed to unlock would hang here.
func TestGate_ReusableAfterRelease(t *testing.T) {
	t.Parallel()

	g := gate.New()

	g.Lock("chat-1")()
	g.Lock("chat-1")()
	g.Lock("chat-1")()
}

// A waiter that arrives while the gate is held still gets in, and gets in AFTER
// the holder leaves. The order is observed through the section itself: the holder
// appends before releasing, the waiter appends after being let in.
func TestGate_WaiterEntersAfterHolderReleases(t *testing.T) {
	t.Parallel()

	g := gate.New()

	release := g.Lock("chat-1")

	var order []string
	entered := make(chan struct{})
	go func() {
		defer close(entered)
		waiterRelease := g.Lock("chat-1")
		defer waiterRelease()
		order = append(order, "waiter")
	}()

	order = append(order, "holder")
	release()
	<-entered

	if len(order) != 2 || order[0] != "holder" || order[1] != "waiter" {
		t.Fatalf("order = %v, want [holder waiter]", order)
	}
}
