package chat_test

import (
	"fmt"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSwitchProvider_LostStartRace_TearsDownOrphanCLI is the retarget of the old
// keyed_mutex / OpenSegment-race test. Both are gone: per-aggregate concurrency is the
// asynx write-path (id,version) optimistic-concurrency control, and there is no longer
// an OpenSegment that can be rejected because "an active segment already exists" — a
// chat holds no process state to conflict over.
//
// What the USECASE still owns, and what this pins deterministically, is the orphan
// teardown. A pure command cannot spawn a process, so the CLI is necessarily live
// BEFORE the runner that describes it can be recorded; if that record fails for any
// reason, the just-spawned CLI must be torn down. A running CLI that nothing in Crowbar
// points at is invisible — it is the state the whole refactor exists to make
// impossible — so the teardown is unconditional, and the original error still surfaces
// (an ErrValidation still classifies as a conflict upstream). We force the failure with
// a store double rather than a timing-dependent goroutine storm: no sleeps, no
// nondeterminism.
func TestSwitchProvider_LostStartRace_TearsDownOrphanCLI(t *testing.T) {
	f, _, rs := newFaultFixture(t)

	chatID, _ := f.spawn(t, "claude")

	rs.failStart = fmt.Errorf("start runner: %w", asynxModels.ErrValidation)

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation,
		"a lost race must surface as a conflict (ErrValidation), not a bare failure")

	// Two CLIs were spawned: term-1 (the original claude) and term-2 (the target codex
	// whose runner could not be recorded). term-2 must have been torn down so no orphan
	// process leaks; term-1 was terminated as the normal outgoing-CLI quit.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-2",
		"the just-spawned CLI whose runner could not be recorded must be torn down")
}
