package registry_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
)

func TestRegistry_Consume_RecognisesAnInjectedDocumentOnce(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "handoff blob")

	assert.True(t, r.Consume("runner-1", "handoff blob"))
	assert.False(t, r.Consume("runner-1", "handoff blob"),
		"the match is one-shot so a user retyping the text later is still recorded")
}

// TestRegistry_Consume_RecognisesTheDocWrappedByTheProvidersOwnTemplate pins
// the live bug: claude's resume pointer reaches the CLI wrapped in
// <system-reminder> tags the descriptor's own template adds — Go registers
// the bare pointer it composed, never the wrapped form. An equality check
// missed that wrapping entirely and let the injected handoff land in the
// ledger as the user's own turn.
func TestRegistry_Consume_RecognisesTheDocWrappedByTheProvidersOwnTemplate(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "[Crowbar] call get_chat_log")

	assert.True(t, r.Consume("runner-1", "<system-reminder>[Crowbar] call get_chat_log</system-reminder>"))
}

func TestRegistry_Consume_IsScopedToItsRunner(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "blob")

	assert.False(t, r.Consume("runner-2", "blob"))
	assert.True(t, r.Consume("runner-1", "blob"))
}

func TestRegistry_SetInjected_DropsEmptyDocumentsSoEmptyTextNeverMatches(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "", "real")

	assert.False(t, r.Consume("runner-1", ""))
	assert.True(t, r.Consume("runner-1", "real"))
}

func TestRegistry_SetInjected_RecordsEveryDocumentHandedToOneSpawn(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "context doc", "pointer message")

	assert.True(t, r.Consume("runner-1", "pointer message"))
	assert.True(t, r.Consume("runner-1", "context doc"))
}

func TestRegistry_Forget_DropsADeadRunnersEntries(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "blob")

	r.Forget("runner-1")

	assert.False(t, r.Consume("runner-1", "blob"))
}

func TestRegistry_Consume_UnknownRunnerIsNotAMatch(t *testing.T) {
	assert.False(t, registry.New().Consume("nobody", "blob"))
}

func TestRegistry_IsSafeUnderConcurrentUse(t *testing.T) {
	r := registry.New()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "runner"
			r.SetInjected(id, "doc")
			r.Consume(id, "doc")
			if i%7 == 0 {
				r.Forget(id)
			}
		}(i)
	}
	wg.Wait()
}
