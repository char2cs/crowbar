package agentjournal

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequireNoActiveRequest_PropagatesARealReadFailure(t *testing.T) {
	err := requireNoActiveRequest(filepath.Join(t.TempDir(), "does-not-exist"), "req-1")

	require.Error(t, err)
}

func TestRequireNoActiveRequest_RejectsAStillDispatchingRecord(t *testing.T) {
	// Begin always runs recoverOrphanedDispatchesLocked immediately before this
	// check, which downgrades every Dispatching record to Uncertain first — so
	// through the public Begin flow this function can never actually observe a
	// still-Dispatching record. It is exercised directly here because the
	// ErrPromptOutcomeUnknown branch is real defensive logic (protecting against
	// a future caller, or a different lock ordering, that reaches this check
	// before a recovery sweep) and must still behave correctly if it is ever hit.
	dir := t.TempDir()
	record := PromptRequest{
		RequestID: "still-dispatching",
		State:     PromptStateDispatching,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	require.NoError(t, writeRecord(dir, record.RequestID+".json", record, promptRequestTempPat, func(string) error { return nil }))

	err := requireNoActiveRequest(dir, "some-other-id")

	assert.ErrorIs(t, err, ErrPromptOutcomeUnknown)
}
