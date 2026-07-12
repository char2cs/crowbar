package agent_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
)

// The Registry is now ONLY the injected-context echo guard. Its placement maps
// (segToChat / sessionToChat / segToSession) and the OnSessionStart reducer that
// mutated them are deleted: they were an in-memory shadow of durable state, mutated
// BEFORE the aggregate commands ran, so a failed command left the registry believing a
// move had happened that had not. Placement now lives in the runner aggregate and the
// reducer is pure (Decide, reducer.go); what remains here has no durable counterpart
// to drift from.

// TestConsumeInjectedContext_RecognisesCrowbarsOwnDocument: a provider whose only
// resume channel is a user message (codex) fires its user-prompt hook with the very
// handoff Crowbar injected. Recording that echo as a ledger turn is what made handoffs
// NEST — the blob became a "user" turn, and the next handoff embedded it inside itself.
func TestConsumeInjectedContext_RecognisesCrowbarsOwnDocument(t *testing.T) {
	r := agent.NewRegistry()
	r.SetInjectedContext("runner-1", "the handoff document", "the pointer message")

	assert.True(t, r.ConsumeInjectedContext("runner-1", "the handoff document"))
	assert.True(t, r.ConsumeInjectedContext("runner-1", "the pointer message"))
	assert.False(t, r.ConsumeInjectedContext("runner-1", "something the user actually typed"))
}

// TestConsumeInjectedContext_IsOneShot: the match consumes the entry, so a user who
// genuinely retypes that same text later is still recorded. The guard must never become
// a permanent content filter.
func TestConsumeInjectedContext_IsOneShot(t *testing.T) {
	r := agent.NewRegistry()
	r.SetInjectedContext("runner-1", "echo me")

	require.True(t, r.ConsumeInjectedContext("runner-1", "echo me"))
	assert.False(t, r.ConsumeInjectedContext("runner-1", "echo me"),
		"a second, genuinely user-sent copy must be recorded")
}

// TestConsumeInjectedContext_IsScopedToItsRunner: two CLIs can be handed the same
// document; consuming one runner's echo must not swallow the other's.
func TestConsumeInjectedContext_IsScopedToItsRunner(t *testing.T) {
	r := agent.NewRegistry()
	r.SetInjectedContext("runner-1", "same doc")
	r.SetInjectedContext("runner-2", "same doc")

	require.True(t, r.ConsumeInjectedContext("runner-1", "same doc"))
	assert.True(t, r.ConsumeInjectedContext("runner-2", "same doc"),
		"the other runner's guard must be untouched")
}

// TestSetInjectedContext_IgnoresEmptyDocuments: a spawn with no handoff and no title
// instruction injects nothing, and an empty hook message must never match it.
func TestSetInjectedContext_IgnoresEmptyDocuments(t *testing.T) {
	r := agent.NewRegistry()
	r.SetInjectedContext("runner-1", "", "")

	assert.False(t, r.ConsumeInjectedContext("runner-1", ""))
}

// TestForgetRunner_DropsTheGuard: the entry is per-spawn state about a LIVE process.
// Once the PTY is gone it means nothing, and it holds a whole handoff document — a
// long-lived daemon spawns a lot of CLIs.
func TestForgetRunner_DropsTheGuard(t *testing.T) {
	r := agent.NewRegistry()
	r.SetInjectedContext("runner-1", "doc")
	r.SetInjectedContext("runner-2", "doc")

	r.ForgetRunner("runner-1")

	assert.False(t, r.ConsumeInjectedContext("runner-1", "doc"), "the dead runner's guard is gone")
	assert.True(t, r.ConsumeInjectedContext("runner-2", "doc"), "and no other runner's is disturbed")

	r.ForgetRunner("runner-never-existed") // idempotent
}

func TestRegistry_ConcurrentNoRace(t *testing.T) {
	r := agent.NewRegistry()
	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.SetInjectedContext("runner", "doc")
			r.ConsumeInjectedContext("runner", "doc")
			if i%10 == 0 {
				r.ForgetRunner("runner")
			}
		}()
	}
	wg.Wait()
}
