package branchreview_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store/wspaths"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// benchReviewHarness holds the real stack needed for BenchmarkBranchReview_Get.
type benchReviewHarness struct {
	uc          branchreview.Usecase
	wsID        string
	featurePath string
}

func newBenchReviewHarness(
	b *testing.B,
) *benchReviewHarness {
	b.Helper()
	ctx := context.Background()

	// ── adapter container (per-entity workspace storage under a temp home) ────
	adapters, err := adapter.New(adapter.WithHomeDir(b.TempDir()))
	require.NoError(b, err)
	b.Cleanup(func() { _ = adapters.Close() })

	// ── event stores (in-memory SQLite) ──────────────────────────────────────
	rtES, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(b, err)
	axRT, err := asynx.New[domain.ReviewThread]().
		WithEventStore(rtES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = axRT.Shutdown(ctx) })

	chatES, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(b, err)
	axChat, err := asynx.New[domain.Chat]().
		WithEventStore(chatES).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = axChat.Shutdown(ctx) })

	// ── GORM DB (the adapter's global view) ───────────────────────────────────
	db := adapters.GlobalView()

	wsAx, err := asynx.New[domain.Workspace]().
		WithEventStore(adapters.WorkspaceES()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = wsAx.Shutdown(ctx) })
	wsPaths, err := wspaths.NewWorkspacePaths(adapters.GlobalView())
	require.NoError(b, err)
	workspaces, err := workspace.New(wsAx, adapters.WorkspaceView(), wsPaths)
	require.NoError(b, err)

	threads, err := reviewthread.New(ctx, axRT, rtES, db, func(domain.ReviewThread) {})
	require.NoError(b, err)

	chats, err := chat.New(ctx, axChat, chatES, db, func(domain.Chat) {})
	require.NoError(b, err)

	repoStore, err := storesqlite.NewFromDB[domain.Repository, string](db)
	require.NoError(b, err)

	// ── real git repo: base branch + feature branch worktree ─────────────────
	repoPath := benchInitRepo(b)
	baseBranch := trimNewline(benchGitRun(b, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))

	// seed a file on base so the three-dot diff has something to compare
	benchCommitInWorktree(b, repoPath, "base.txt", "base content\n", "base: seed file")

	// create a feature branch worktree with one added file
	featurePath := filepath.Join(b.TempDir(), "feature-wt")
	benchGitRun(b, repoPath, "worktree", "add", "-b", "feature/bench-review", featurePath)
	benchCommitInWorktree(b, featurePath, "feature.txt", "feature content\n", "feature: add file")

	// ── persist domain objects ────────────────────────────────────────────────
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
	}, fixedNow)
	require.NoError(b, err)

	// seed one review thread so the ListByWorkspace read model is non-trivial
	_, err = threads.Open(ctx, reviewthread.OpenInput{
		ID:         "rt-bench-1",
		WsID:       wsID,
		FilePath:   "feature.txt",
		LineNumber: 1,
		MessageID:  "msg-bench-1",
		Body:       "benchmark comment",
	}, fixedNow)
	require.NoError(b, err)

	// seed one chat
	_, err = chats.Create(ctx, "chat-bench-1", wsID, "bench chat", fixedNow)
	require.NoError(b, err)

	uc := branchreview.New(
		workspaces,
		threads,
		chats,
		repoStore,
		enginegit.New(),
		func() time.Time { return fixedNow },
	)

	return &benchReviewHarness{
		uc:          uc,
		wsID:        wsID,
		featurePath: featurePath,
	}
}

// BenchmarkBranchReview_Get measures the composite assembly hot path:
// git RangeDiff + ReviewThread.ListByWorkspace + Chat.ListByWorkspace.
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
		if review.Diff.Files == nil && len(review.Threads) == 0 {
			// prevent the compiler from optimising away the call
			b.Fatal("unexpected empty result")
		}
	}
}
