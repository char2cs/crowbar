package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCatalogRunsEnforcesOneDaemonWideProcessBudget(t *testing.T) {
	runs := newCatalogRuns()
	releases := make([]func(), 0, maxCatalogProcesses)
	for range maxCatalogProcesses {
		release, err := runs.acquireProcess(context.Background())
		require.NoError(t, err)
		releases = append(releases, release)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := runs.acquireProcess(waitCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"a fifth command must wait on the shared budget and remain cancellation-aware")

	releases[0]()
	replacement, err := runs.acquireProcess(context.Background())
	require.NoError(t, err, "releasing one process slot must admit exactly one waiter")
	replacement()
	for _, release := range releases[1:] {
		release()
	}
}
