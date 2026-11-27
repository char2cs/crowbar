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

			assert.Equal(t, []string{"parent", "childA", "childB"}, syncer.SyncedIDs)
		})
	}
}

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
