package agent_test

import (
	"context"
	"fmt"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSwitchProvider_LostOpenSegmentRace_TearsDownOrphanCLI is the retarget of
// the old keyed_mutex regression test. The keyed_mutex is gone: per-aggregate
// concurrency is now the asynx write-path (id,version) optimistic-concurrency
// control, whose real racing behavior — two concurrent OpenSegments to one
// chat, exactly one commits, the loser is rejected with ErrValidation — is
// proved deterministically at the repository layer
// (agentchat.TestAgentChat_ConcurrentOpenSegment_OneWins).
//
// What the USECASE adds on top, and what this test pins deterministically, is
// the orphan teardown: because a PTY spawn cannot live inside a pure command,
// SwitchProvider spawns the target CLI and THEN calls OpenSegment; if
// OpenSegment loses the version race (an active segment already exists —
// asynx ErrValidation), the just-spawned CLI must be torn down
// (TerminateGraceful) so no orphan process leaks, and the conflict must surface
// (still classifying as ErrValidation). We force the lost race with a store
// double rather than a timing-dependent goroutine storm — no sleeps, no
// nondeterminism.
//
// The old test's other assertion — "never two active segments per
// crowbarSegID under a storm of concurrent session_start hooks" — is dropped:
// a single vendor CLI fires session_start sequentially, and the aggregate's
// ≤1-active-segment invariant is now a per-chat command invariant
// (OpenSegment.Validate), not a per-crowbarSegID one the usecase serializes.
func TestSwitchProvider_LostOpenSegmentRace_TearsDownOrphanCLI(t *testing.T) {
	f, fs := newFaultFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	// Simulate losing the OpenSegment version race: an active segment already
	// exists, so the command's Validate rejects this open (wrapping
	// ErrValidation), exactly as the OCC retry surfaces a real race's loser.
	fs.failOpenSeg = fmt.Errorf("open segment: active segment exists: %w", asynxModels.ErrValidation)

	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation,
		"a lost OpenSegment race must surface as a conflict (ErrValidation), not a bare failure")

	// Two CLIs were spawned: term-1 (original claude) and term-2 (the target
	// codex whose OpenSegment lost). term-2 must have been torn down so no orphan
	// process leaks; term-1 was terminated as the normal outgoing-CLI quit.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-2",
		"the just-spawned CLI whose OpenSegment lost the race must be torn down")
}
