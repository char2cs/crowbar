package inflight_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
)

// Every constructor here returns a SEPARATE instance, and the package's contract
// is that the chat usecase calls each of them exactly once. This test pins the
// half of that contract the package can enforce: nothing is shared behind the
// caller's back, so a second call really is a second, empty world — which is why
// a second call in production is a silent wedge rather than a harmless one.
func TestConstructorsReturnIndependentInstances(t *testing.T) {
	t.Parallel()

	t.Run("gate", func(t *testing.T) {
		t.Parallel()
		first, second := inflight.NewGate(), inflight.NewGate()
		release := first.Lock("chat-1")
		defer release()
		// A shared gate would deadlock here instead of returning.
		second.Lock("chat-1")()
	})

	t.Run("hooks", func(t *testing.T) {
		t.Parallel()
		first, second := inflight.NewHooks(), inflight.NewHooks()
		if err := first.Register("runner-1"); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if handled, _ := second.Enqueue("runner-1", "claude", "turn_start", nil); handled {
			t.Fatal("a second barrier absorbed a hook for a runner registered on the first")
		}
	})
}

// The face's aliases must name the same types the constructors return, or a
// component that stores an inflight.Gate could not be handed one.
func TestAliasesNameTheConstructedTypes(t *testing.T) {
	t.Parallel()

	// The assignments are what is under test: each alias must name the type its
	// constructor returns, or a component that stores an inflight.Gate could not be
	// handed one.
	var (
		gate  *inflight.Gate
		hooks *inflight.Hooks
		hook  inflight.Hook
	)
	gate, hooks = inflight.NewGate(), inflight.NewHooks()
	if gate == nil || hooks == nil {
		t.Fatal("a constructor returned nil")
	}
	if hook.Provider != "" || hook.RawPayload != nil {
		t.Fatal("the zero Hook is not zero")
	}
}
