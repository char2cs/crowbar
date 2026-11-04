package git_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitUsecase_OperationContinue_ClearsStickyConflicts(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	git.AnyWriteOK()

	// A successful continue fully resolves the in-progress operation in this
	// worktree, so the sticky pr-conflicts status must be cleared.
	require.NoError(t, uc.OperationContinue(ctx, "w1", now))
	assert.True(t, syncer.Resolved, "a successful continue must clear sticky pr-conflicts")
	assert.Equal(t, "w1", syncer.ResolvedID)
}

func TestGitUsecase_OperationAbort_KeepsConflicts(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	git.AnyWriteOK()

	// Abort discards the resolution attempt; the branch is still conflicting, so
	// pr-conflicts must NOT be cleared.
	require.NoError(t, uc.OperationAbort(ctx, "w1", now))
	assert.False(t, syncer.Resolved, "abort must not clear pr-conflicts")
}
