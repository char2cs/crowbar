package catalog_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/catalog"
)

func TestAcquireProcess_EnforcesOneDaemonWideBudget(t *testing.T) {
	t.Parallel()

	runs := catalog.New()
	releases := make([]func(), 0, 4)
	for range 4 {
		release, err := runs.AcquireProcess(context.Background())
		require.NoError(t, err)
		releases = append(releases, release)
	}

	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runs.AcquireProcess(waitCtx)
	require.ErrorIs(t, err, context.Canceled,
		"a fifth probe must wait on the shared budget and remain cancellation-aware")

	releases[0]()
	replacement, err := runs.AcquireProcess(context.Background())
	require.NoError(t, err, "releasing one process slot must admit exactly one waiter")
	replacement()
	for _, release := range releases[1:] {
		release()
	}
}

func TestAcquireProcess_ReleaseIsIdempotent(t *testing.T) {
	t.Parallel()

	runs := catalog.New()
	release, err := runs.AcquireProcess(context.Background())
	require.NoError(t, err)

	release()
	release()
	release()

	var releases []func()
	for range 4 {
		r, err := runs.AcquireProcess(context.Background())
		require.NoError(t, err)
		releases = append(releases, r)
	}
	done, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = runs.AcquireProcess(done)
	assert.ErrorIs(t, err, context.Canceled, "the budget grew past four")
	for _, r := range releases {
		r()
	}
}

func TestStart_ASecondProbeCancelsTheFirst(t *testing.T) {
	t.Parallel()

	runs := catalog.New()

	first, finishFirst := runs.Start(context.Background(), "chat-1")
	defer finishFirst()
	second, finishSecond := runs.Start(context.Background(), "chat-1")
	defer finishSecond()

	require.ErrorIs(t, first.Err(), context.Canceled)
	assert.NoError(t, second.Err(), "the probe that replaced it is still live")
}

func TestStart_ChatsDoNotCancelEachOther(t *testing.T) {
	t.Parallel()

	runs := catalog.New()

	mine, finishMine := runs.Start(context.Background(), "chat-1")
	defer finishMine()
	theirs, finishTheirs := runs.Start(context.Background(), "chat-2")
	defer finishTheirs()

	assert.NoError(t, mine.Err())
	assert.NoError(t, theirs.Err())
}

func TestStart_ASupersededProbeFinishingLateDoesNotEvictItsReplacement(t *testing.T) {
	t.Parallel()

	runs := catalog.New()

	_, finishFirst := runs.Start(context.Background(), "chat-1")
	second, finishSecond := runs.Start(context.Background(), "chat-1")
	defer finishSecond()

	finishFirst()

	third, finishThird := runs.Start(context.Background(), "chat-1")
	defer finishThird()

	require.ErrorIs(t, second.Err(), context.Canceled,
		"the second probe survived the third starting; its registry entry had been evicted")
	assert.NoError(t, third.Err())
}

func TestStart_FinishCancelsTheProbe(t *testing.T) {
	t.Parallel()

	runs := catalog.New()

	ctx, finish := runs.Start(context.Background(), "chat-1")
	finish()

	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

func TestStart_CancellingTheParentCancelsTheProbe(t *testing.T) {
	t.Parallel()

	runs := catalog.New()
	parent, cancelParent := context.WithCancel(context.Background())

	ctx, finish := runs.Start(parent, "chat-1")
	defer finish()
	cancelParent()

	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}
