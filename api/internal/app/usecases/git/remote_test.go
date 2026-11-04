package git_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gituc "github.com/char2cs/crowbar/api/internal/app/usecases/git"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// After a pull/fetch, the git usecase must cascade the working-tree resync to the
// pulled workspace's DIRECT children (their sidebar diff is measured against the
// parent branch's live merge-base, so moving the parent's tip stales them). It
// must resync the parent itself plus every direct child, skip non-children and the
// self-loop row, and never touch grandchildren.
func TestGitUsecase_PullFetch_CascadeResyncsDirectChildren(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   func(uc gituc.Usecase, ctx context.Context, now time.Time) error
	}{
		{"pull", func(uc gituc.Usecase, ctx context.Context, now time.Time) error {
			return uc.Pull(ctx, "parent", "merge", now)
		}},
		{"fetch", func(uc gituc.Usecase, ctx context.Context, now time.Time) error {
			return uc.Fetch(ctx, "parent", now)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git, syncer, uc := newGitUsecase(t)
			ctx := context.Background()
			now := time.Unix(1000, 0)
			git.AnyWriteOK()
			syncer.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
				return []domain.Workspace{
					{ID: "parent", ParentID: ""},
					{ID: "childA", ParentID: "parent"},
					{ID: "childB", ParentID: "parent"},
					{ID: "grandchild", ParentID: "childA"},
					{ID: "unrelated", ParentID: "other"},
					{ID: "selfloop", ParentID: "selfloop"},
				}, nil
			}

			require.NoError(t, tc.op(uc, ctx, now))

			// The pulled workspace is resynced by mutate; both direct children by the
			// cascade. Grandchild, unrelated, and the self-loop are NOT resynced.
			assert.Equal(t, []string{"parent", "childA", "childB"}, syncer.SyncedIDs)
		})
	}
}

// A List failure while enumerating children must NOT fail the pull the user just
// completed — the cascade is best-effort. The parent's own resync still stands.
func TestGitUsecase_Pull_ChildListFailure_DoesNotFailPull(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	git.AnyWriteOK()
	syncer.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return nil, errors.New("boom")
	}

	require.NoError(t, uc.Pull(ctx, "parent", "merge", now))
	assert.Equal(t, []string{"parent"}, syncer.SyncedIDs, "only the parent's own resync ran")
}

// One child's resync failure must NOT fail the pull nor block a sibling's resync.
func TestGitUsecase_Pull_ChildSyncFailure_ContinuesSiblings(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	git.AnyWriteOK()
	syncer.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		return []domain.Workspace{
			{ID: "parent"},
			{ID: "childA", ParentID: "parent"},
			{ID: "childB", ParentID: "parent"},
		}, nil
	}
	syncer.SyncFn = func(_ context.Context, id string, _ time.Time) (domain.Workspace, error) {
		if id == "childA" {
			return domain.Workspace{}, errors.New("boom")
		}
		return domain.Workspace{ID: id}, nil
	}

	require.NoError(t, uc.Pull(ctx, "parent", "merge", now))
	assert.Equal(t, []string{"parent", "childA", "childB"}, syncer.SyncedIDs,
		"a failed child sync must not stop the sibling from syncing")
}

// A failed pull must NOT cascade: children stay untouched when the pull itself
// errored (no ref moved).
func TestGitUsecase_Pull_EngineError_NoCascade(t *testing.T) {
	git, syncer, uc := newGitUsecase(t)
	ctx := context.Background()
	git.PullFn = func(_ context.Context, _, _ string) error { return errors.New("boom") }
	listed := false
	syncer.ListFn = func(_ context.Context) ([]domain.Workspace, error) {
		listed = true
		return nil, nil
	}

	err := uc.Pull(ctx, "parent", "merge", time.Now())
	require.Error(t, err)
	assert.False(t, syncer.Synced, "a failed pull must not resync")
	assert.False(t, listed, "a failed pull must not enumerate children")
}
