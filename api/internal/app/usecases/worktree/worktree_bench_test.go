package worktree_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// benchHarness holds the wired-up usecase and handles needed per benchmark run.
type benchHarness struct {
	uc         worktree.Usecase
	workspaces workspace.Workspace
	repoPath   string
	baseBranch string
	parentID   string
	repoID     string
	projectID  string
}

func newBenchHarness(
	b *testing.B,
) *benchHarness {
	b.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(b, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(b, err)
	b.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(b, err)
	workspaces, err := workspace.New(ax, db, func(domain.Workspace) {})
	require.NoError(b, err)

	repos, err := storesqlite.NewFromDB[domain.Repository, string](db)
	require.NoError(b, err)

	repoPath := benchInitRepo(b)
	repoID := "r1"
	projectID := "p1"
	baseBranch := benchBranchName(b, repoPath)

	require.NoError(b, repos.Save(context.Background(), domain.Repository{
		ID:        repoID,
		ProjectID: projectID,
		Name:      "repo",
		Path:      repoPath,
		RemoteURL: "https://github.com/test/bench-repo.git",
	}))

	prov := &stubProvider{}
	uc := worktree.New(
		workspaces,
		enginegit.New(),
		prov,
		repos,
		func() time.Time { return time.Now() },
		func() (string, error) { return b.TempDir(), nil },
	)

	parentID := "w-parent"
	_, err = workspaces.Create(context.Background(), workspace.CreateInput{
		ID:           parentID,
		RepoID:       repoID,
		ProjectID:    projectID,
		Branch:       baseBranch,
		WorktreePath: repoPath,
	}, time.Now())
	require.NoError(b, err)

	return &benchHarness{
		uc:         uc,
		workspaces: workspaces,
		repoPath:   repoPath,
		baseBranch: baseBranch,
		parentID:   parentID,
		repoID:     repoID,
		projectID:  projectID,
	}
}

func benchInitRepo(
	b *testing.B,
) string {
	b.Helper()
	dir := b.TempDir()
	benchGitRun(b, dir, "init")
	benchGitRun(b, dir, "config", "user.email", "t@t")
	benchGitRun(b, dir, "config", "user.name", "t")
	benchGitRun(b, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

func benchBranchName(
	b *testing.B,
	dir string,
) string {
	b.Helper()
	out := benchGitRun(b, dir, "rev-parse", "--abbrev-ref", "HEAD")
	return trimNewline(out)
}

func benchGitRun(
	b *testing.B,
	dir string,
	args ...string,
) string {
	b.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		cmd.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(b, err, string(out))
	return string(out)
}

func benchCommitInWorktree(
	b *testing.B,
	worktree string,
	file string,
	content string,
	msg string,
) {
	b.Helper()
	path := filepath.Join(worktree, file)
	cmd := exec.Command("sh", "-c", "printf '%s' "+shellQuote(content)+" > "+shellQuote(path))
	require.NoError(b, cmd.Run())
	benchGitRun(b, worktree, "add", file)
	benchGitRun(b, worktree, "commit", "-m", msg)
}

func (h *benchHarness) createChild(
	b *testing.B,
	branch string,
	parentID string,
	parentBranch string,
) domain.Workspace {
	b.Helper()
	in := worktree.CreateChildInput{
		RepoID:       h.repoID,
		ProjectID:    h.projectID,
		RepoPath:     h.repoPath,
		RemoteURL:    "https://github.com/test/bench-repo.git",
		Branch:       branch,
		ParentID:     parentID,
		ParentBranch: parentBranch,
	}
	child, err := h.uc.CreateChild(context.Background(), in)
	require.NoError(b, err)
	return child
}

// BenchmarkMergeIntoParent measures the local child→parent merge hot path
// (07 §6 — a broadcast fires on every mutation). Each iteration creates a fresh
// child branch with one commit then runs MergeIntoParent; setup is excluded from
// the timed window via b.StopTimer/b.StartTimer.
func BenchmarkMergeIntoParent(b *testing.B) {
	ctx := context.Background()

	for i := range b.N {
		b.StopTimer()
		h := newBenchHarness(b)
		branch := fmt.Sprintf("feature/bench-merge-%d", i)
		child := h.createChild(b, branch, h.parentID, h.baseBranch)
		benchCommitInWorktree(b, child.WorktreePath, fmt.Sprintf("bench-%d.txt", i), "bench\n", "bench commit")
		b.StartTimer()

		_, err := h.uc.MergeIntoParent(ctx, child.ID, gitdomain.MergeStrategyMerge)
		if err != nil {
			b.Fatalf("MergeIntoParent: %v", err)
		}
	}
}

// BenchmarkReparent measures the leaf re-parent hot path (07 §4, §6). Each
// iteration creates a target parent-b branch and a leaf child, then re-parents
// the child onto parent-b; setup is excluded from the timed window.
func BenchmarkReparent(b *testing.B) {
	ctx := context.Background()

	for i := range b.N {
		b.StopTimer()
		h := newBenchHarness(b)
		parentB := h.createChild(b, fmt.Sprintf("feature/pb-%d", i), h.parentID, h.baseBranch)
		benchCommitInWorktree(b, parentB.WorktreePath, fmt.Sprintf("pb-%d.txt", i), "parent-b\n", "parent-b commit")
		child := h.createChild(b, fmt.Sprintf("feature/child-%d", i), h.parentID, h.baseBranch)
		benchCommitInWorktree(b, child.WorktreePath, fmt.Sprintf("child-%d.txt", i), "child\n", "child commit")
		b.StartTimer()

		_, err := h.uc.Reparent(ctx, child.ID, parentB.ID)
		if err != nil {
			b.Fatalf("Reparent: %v", err)
		}
	}
}
