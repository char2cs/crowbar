package branchreview_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

type benchReviewHarness struct {
	uc          branchreview.Usecase
	wsID        string
	featurePath string
	repoPath    string
	engine      enginegit.Engine
}

func newBenchReviewHarness(
	b *testing.B,
) *benchReviewHarness {
	b.Helper()
	ctx := context.Background()

	adapters, err := adapter.New(adapter.WithHomeDir(b.TempDir()))
	require.NoError(b, err)
	b.Cleanup(func() { _ = adapters.Close() })

	rtES, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(b, err)
	rtRM, err := reviewthread.OpenReadModel(adapters.ReviewThreadView())
	require.NoError(b, err)
	rtB := asynx.New[domain.ReviewThread]().
		WithEventStore(rtES).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	reviewthread.Subscriber(rtRM)(rtB)
	axRT, err := rtB.Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = axRT.Shutdown(ctx) })

	db := adapters.GlobalView()

	rm, err := workspace.OpenReadModel(adapters.WorkspaceView())
	require.NoError(b, err)
	axb := asynx.New[domain.Workspace]().
		WithEventStore(adapters.WorkspaceES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	workspace.Subscriber(rm)(axb)
	wsAx, err := axb.Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = wsAx.Shutdown(ctx) })
	workspaces, err := workspace.New(wsAx, adapters.WorkspaceES(), rm)
	require.NoError(b, err)

	threads, err := reviewthread.New(axRT, rtES, rtRM, func(domain.ReviewThread) {})
	require.NoError(b, err)

	repoStore, err := storesqlite.NewFromDB[domain.Repository, string](db)
	require.NoError(b, err)

	repoPath := benchInitRepo(b)
	baseBranch := trimNewline(benchGitRun(b, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))

	benchCommitInWorktree(b, repoPath, "base.txt", "base content\n", "base: seed file")

	featurePath := filepath.Join(b.TempDir(), "feature-wt")
	benchGitRun(b, repoPath, "worktree", "add", "-b", "feature/bench-review", featurePath)
	benchCommitInWorktree(b, featurePath, "feature.txt", "feature content\n", "feature: add file")

	repoID := "bench-repo"
	projectID := "bench-project"
	wsID := "bench-ws"
	fixedNow := time.Unix(1_000_000, 0).UTC()

	require.NoError(b, repoStore.Save(ctx, domain.Repository{
		ID:            repoID,
		ProjectID:     projectID,
		Name:          "bench-repo",
		Path:          repoPath,
		DefaultBranch: baseBranch,
	}))

	_, err = workspaces.Create(ctx, workspace.CreateInput{
		ID:           wsID,
		RepoID:       repoID,
		ProjectID:    projectID,
		Branch:       "feature/bench-review",
		WorktreePath: featurePath,
		Provisioning: domain.WorkspaceProvisioned,
	}, fixedNow)
	require.NoError(b, err)

	_, err = threads.Open(ctx, reviewthread.OpenInput{
		ID:         "rt-bench-1",
		WsID:       wsID,
		FilePath:   "feature.txt",
		LineNumber: 1,
		MessageID:  "msg-bench-1",
		Body:       "benchmark comment",
	}, fixedNow)
	require.NoError(b, err)

	gitEngine := enginegit.New()
	uc := branchreview.New(
		workspaces,
		threads,
		repoStore,
		gitEngine,
		func() time.Time { return fixedNow },
	)

	return &benchReviewHarness{
		uc:          uc,
		wsID:        wsID,
		featurePath: featurePath,
		repoPath:    repoPath,
		engine:      gitEngine,
	}
}

func BenchmarkBranchReview_Get(
	b *testing.B,
) {
	ctx := context.Background()
	h := newBenchReviewHarness(b)
	b.ResetTimer()

	for range b.N {
		review, err := h.uc.Get(ctx, h.wsID)
		if err != nil {
			b.Fatalf("BranchReview.Get: %v", err)
		}
		if len(review.Threads) == 0 {
			b.Fatal("unexpected empty result")
		}
	}
}
