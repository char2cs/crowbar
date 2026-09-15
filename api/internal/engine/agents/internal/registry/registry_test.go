package registry_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
)

func TestRegistry_ConsumePrefix_RecognisesAnInjectedDocumentOnce(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "handoff blob")

	remainder, found := r.ConsumePrefix("runner-1", "handoff blob")
	assert.True(t, found)
	assert.Empty(t, remainder, "a bare echo with nothing else riding it leaves no remainder")

	remainder, found = r.ConsumePrefix("runner-1", "handoff blob")
	assert.False(t, found, "the match is one-shot so a user retyping the text later is still recorded")
	assert.Equal(t, "handoff blob", remainder, "an unmatched text is returned unchanged")
}

// TestRegistry_ConsumePrefix_RecognisesTheDocWrappedByTheProvidersOwnTemplate
// pins the live bug: claude's resume pointer reaches the CLI wrapped in
// <system-reminder> tags the descriptor's own template adds — Go registers
// the bare pointer it composed, never the wrapped form. An equality check
// missed that wrapping entirely and let the injected handoff land in the
// ledger as the user's own turn.
func TestRegistry_ConsumePrefix_RecognisesTheDocWrappedByTheProvidersOwnTemplate(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "[Crowbar] call get_chat_log")

	remainder, found := r.ConsumePrefix("runner-1", "<system-reminder>[Crowbar] call get_chat_log</system-reminder>")
	assert.True(t, found)
	assert.Empty(t, remainder)
}

// TestRegistry_ConsumePrefix_ARealPromptRidingTheSameSpawnIsReturnedAsTheRemainder
// is the fix for the live bug this session's own merge fix (mergeLeadingPositional,
// spawnsteps.go) created one layer downstream: when a real user prompt is folded
// directly ahead of — behind, in the actual argv order — the injected context into
// ONE positional argv token, the hook echoes the WHOLE combined string back. A
// bare "was this injected" boolean has nowhere to put "yes, AND here is the real
// part" — it either suppresses the whole turn (losing the user's real words from
// the ledger) or reports no match at all (recording Crowbar's own preamble as
// what the user typed). The remainder is what makes both wrong outcomes avoidable.
func TestRegistry_ConsumePrefix_ARealPromptRidingTheSameSpawnIsReturnedAsTheRemainder(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "<system-reminder>gap content</system-reminder>")

	remainder, found := r.ConsumePrefix("runner-1", "<system-reminder>gap content</system-reminder>\n\nwhat did I miss?")

	assert.True(t, found)
	assert.Equal(t, "what did I miss?", remainder,
		"only the user's own real text should remain once the injected preamble and its separator are removed")
}

// TestRegistry_ConsumePrefix_TrimsSurroundingWhitespaceFromTheRemainder guards
// against a cosmetic regression that would otherwise leak into the ledger and
// the derived chat title: mergeLeadingPositional's exact "\n\n" separator, plus
// any incidental whitespace either side of it, must never survive into the
// remainder.
func TestRegistry_ConsumePrefix_TrimsSurroundingWhitespaceFromTheRemainder(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "context")

	remainder, found := r.ConsumePrefix("runner-1", "  context  \n\n  what did I miss?  ")

	assert.True(t, found)
	assert.Equal(t, "what did I miss?", remainder)
}

func TestRegistry_ConsumePrefix_IsScopedToItsRunner(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "blob")

	_, found := r.ConsumePrefix("runner-2", "blob")
	assert.False(t, found)
	_, found = r.ConsumePrefix("runner-1", "blob")
	assert.True(t, found)
}

func TestRegistry_SetInjected_DropsEmptyDocumentsSoEmptyTextNeverMatches(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "", "real")

	_, found := r.ConsumePrefix("runner-1", "")
	assert.False(t, found)
	_, found = r.ConsumePrefix("runner-1", "real")
	assert.True(t, found)
}

func TestRegistry_SetInjected_RecordsEveryDocumentHandedToOneSpawn(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "context doc", "pointer message")

	_, found := r.ConsumePrefix("runner-1", "pointer message")
	assert.True(t, found)
	_, found = r.ConsumePrefix("runner-1", "context doc")
	assert.True(t, found)
}

func TestRegistry_Forget_DropsADeadRunnersEntries(t *testing.T) {
	r := registry.New()
	r.SetInjected("runner-1", "blob")

	r.Forget("runner-1")

	_, found := r.ConsumePrefix("runner-1", "blob")
	assert.False(t, found)
}

func TestRegistry_ConsumePrefix_UnknownRunnerIsNotAMatch(t *testing.T) {
	_, found := registry.New().ConsumePrefix("nobody", "blob")
	assert.False(t, found)
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
			r.ConsumePrefix(id, "doc")
			if i%7 == 0 {
				r.Forget(id)
			}
		}(i)
	}
	wg.Wait()
}
