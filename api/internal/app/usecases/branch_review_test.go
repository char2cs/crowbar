package usecases_test

import (
	"context"
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestBranchReview_Get_MissingWorkspace_IsNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, asynxModels.ErrNotFound
		},
	}
	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Get(ctx, "nope")
	require.ErrorIs(t, err, apperr.ErrNotFound)
}

func TestBranchReview_Get_InternalGitError_IsNotNotFound(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "repo1", Branch: "feature", WorktreePath: "/wt"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()
	_ = repoStore.Save(ctx, domain.Repository{ID: "repo1", DefaultBranch: "main"})
	gitEng := &mockGitEngine{
		RangeDiffFn: func(_ context.Context, _, _, _ string) (gitdomain.MultiFileDiff, error) {
			return gitdomain.MultiFileDiff{}, errors.New("git: not a repository")
		},
	}
	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, repoStore, gitEng)

	_, err := uc.Get(ctx, "ws1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, apperr.ErrNotFound)
}

// --- local workspace mock ---

type mockWorkspace struct {
	GetFn              func(ctx context.Context, id string) (domain.Workspace, error)
	SetMergeStrategyFn func(ctx context.Context, id string, s gitdomain.MergeStrategy) (domain.Workspace, error)
}

func (m *mockWorkspace) Get(ctx context.Context, id string) (domain.Workspace, error) {
	return m.GetFn(ctx, id)
}

func (m *mockWorkspace) SetMergeStrategy(
	ctx context.Context,
	id string,
	s gitdomain.MergeStrategy,
) (domain.Workspace, error) {
	return m.SetMergeStrategyFn(ctx, id, s)
}

func (m *mockWorkspace) Create(ctx context.Context, in workspace.CreateInput, now time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) SyncWorkingTreeState(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) SyncProviderState(ctx context.Context, in workspace.ProviderInput, now time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) TouchActivity(ctx context.Context, id string, now time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) Reparent(ctx context.Context, id, parentID, forkPointSha string, now time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) UpdateForkPoint(ctx context.Context, id, forkPointSha string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) SetPendingMerge(ctx context.Context, id string, s gitdomain.MergeStrategy, target string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) ClearPendingMerge(ctx context.Context, id string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}
func (m *mockWorkspace) Delete(ctx context.Context, id string) error { return nil }
func (m *mockWorkspace) List(ctx context.Context) ([]domain.Workspace, error) {
	return nil, nil
}

var _ workspace.Workspace = (*mockWorkspace)(nil)

// --- local reviewthread mock ---

type mockReviewThread struct {
	OpenFn            func(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error)
	ReplyFn           func(ctx context.Context, id, messageID, body string, now time.Time) (domain.ReviewThread, error)
	ResolveFn         func(ctx context.Context, id string) (domain.ReviewThread, error)
	ReopenFn          func(ctx context.Context, id string) (domain.ReviewThread, error)
	ListByWorkspaceFn func(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
}

func (m *mockReviewThread) Open(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error) {
	return m.OpenFn(ctx, in, now)
}

func (m *mockReviewThread) Reply(ctx context.Context, id, messageID, body string, now time.Time) (domain.ReviewThread, error) {
	return m.ReplyFn(ctx, id, messageID, body, now)
}

func (m *mockReviewThread) Resolve(ctx context.Context, id string) (domain.ReviewThread, error) {
	return m.ResolveFn(ctx, id)
}

func (m *mockReviewThread) Reopen(ctx context.Context, id string) (domain.ReviewThread, error) {
	return m.ReopenFn(ctx, id)
}

func (m *mockReviewThread) Get(ctx context.Context, id string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (m *mockReviewThread) List(ctx context.Context) ([]domain.ReviewThread, error) {
	return nil, nil
}

func (m *mockReviewThread) ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error) {
	return m.ListByWorkspaceFn(ctx, wsID)
}

var _ reviewthread.ReviewThread = (*mockReviewThread)(nil)

// --- local chat mock ---

type mockChat struct {
	ListByWorkspaceFn func(ctx context.Context, wsID string) ([]domain.Chat, error)
}

func (m *mockChat) Create(ctx context.Context, id, wsID, title string, now time.Time) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (m *mockChat) Fork(ctx context.Context, id, wsID, parentID, title string, now time.Time) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (m *mockChat) Rename(ctx context.Context, id, title string) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (m *mockChat) Delete(ctx context.Context, id string, now time.Time) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (m *mockChat) ResetIdle(ctx context.Context, id string) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (m *mockChat) SetAgentRunning(ctx context.Context, id string) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (m *mockChat) Get(ctx context.Context, id string) (domain.Chat, error) {
	return domain.Chat{}, nil
}
func (m *mockChat) List(ctx context.Context) ([]domain.Chat, error) { return nil, nil }
func (m *mockChat) ListByWorkspace(ctx context.Context, wsID string) ([]domain.Chat, error) {
	return m.ListByWorkspaceFn(ctx, wsID)
}

// --- local git engine mock (only RangeDiff is exercised by these tests) ---

type mockGitEngine struct {
	RangeDiffFn func(ctx context.Context, repoPath, base, branch string) (gitdomain.MultiFileDiff, error)
}

func (g *mockGitEngine) RangeDiff(ctx context.Context, repoPath, base, branch string) (gitdomain.MultiFileDiff, error) {
	return g.RangeDiffFn(ctx, repoPath, base, branch)
}

func (g *mockGitEngine) Status(ctx context.Context, repoPath string) (gitdomain.GitStatus, error) {
	return gitdomain.GitStatus{}, nil
}

func (g *mockGitEngine) Diff(ctx context.Context, repoPath string, staged bool) ([]gitdomain.FileDiff, error) {
	return nil, nil
}

func (g *mockGitEngine) CommitDiff(ctx context.Context, repoPath, sha string) (gitdomain.MultiFileDiff, error) {
	return gitdomain.MultiFileDiff{}, nil
}

func (g *mockGitEngine) Log(ctx context.Context, repoPath string, limit, skip int) ([]gitdomain.Commit, error) {
	return nil, nil
}

func (g *mockGitEngine) Blame(ctx context.Context, repoPath, filePath string) ([]gitdomain.BlameEntry, error) {
	return nil, nil
}

func (g *mockGitEngine) Branches(ctx context.Context, repoPath string) ([]gitdomain.Branch, error) {
	return nil, nil
}

func (g *mockGitEngine) Stashes(ctx context.Context, repoPath string) ([]gitdomain.Stash, error) {
	return nil, nil
}

func (g *mockGitEngine) StageFile(ctx context.Context, repoPath, filePath string) error { return nil }

func (g *mockGitEngine) StageHunk(ctx context.Context, repoPath, filePath, hunkID string) error {
	return nil
}

func (g *mockGitEngine) UnstageFile(ctx context.Context, repoPath, filePath string) error { return nil }

func (g *mockGitEngine) UnstageHunk(ctx context.Context, repoPath, filePath, hunkID string) error {
	return nil
}

func (g *mockGitEngine) Discard(ctx context.Context, repoPath, filePath string) error { return nil }

func (g *mockGitEngine) Commit(ctx context.Context, repoPath, subject, body string) error { return nil }

func (g *mockGitEngine) Push(ctx context.Context, repoPath string) error { return nil }

func (g *mockGitEngine) Fetch(ctx context.Context, repoPath string) error { return nil }

func (g *mockGitEngine) Pull(ctx context.Context, repoPath, mode string) error { return nil }

func (g *mockGitEngine) CreateBranch(ctx context.Context, repoPath, name, source string, switchTo bool) error {
	return nil
}

func (g *mockGitEngine) RenameBranch(ctx context.Context, repoPath, oldName, newName string) error {
	return nil
}

func (g *mockGitEngine) DeleteBranch(ctx context.Context, repoPath, name string) error {
	return nil
}

func (g *mockGitEngine) ForceDeleteBranch(ctx context.Context, repoPath, name string) error {
	return nil
}

func (g *mockGitEngine) SwitchBranch(ctx context.Context, repoPath, name string) error { return nil }

func (g *mockGitEngine) StashPush(ctx context.Context, repoPath, message string) error { return nil }

func (g *mockGitEngine) StashApply(ctx context.Context, repoPath, id string) error { return nil }

func (g *mockGitEngine) StashPop(ctx context.Context, repoPath, id string) error { return nil }

func (g *mockGitEngine) StashDrop(ctx context.Context, repoPath, id string) error { return nil }

func (g *mockGitEngine) Reset(ctx context.Context, repoPath, mode, commit string) error { return nil }

func (g *mockGitEngine) Merge(ctx context.Context, repoPath, branch string) error { return nil }

func (g *mockGitEngine) Rebase(ctx context.Context, repoPath, onto string) error { return nil }

func (g *mockGitEngine) ConflictedFiles(ctx context.Context, repoPath string) ([]string, error) {
	return nil, nil
}

func (g *mockGitEngine) ConflictHunks(ctx context.Context, repoPath, filePath string) ([]gitdomain.ConflictHunk, error) {
	return nil, nil
}

func (g *mockGitEngine) ResolveHunk(ctx context.Context, repoPath, filePath, hunkID string, resolution gitdomain.ConflictResolution, resolvedContent string) error {
	return nil
}
func (g *mockGitEngine) OperationContinue(ctx context.Context, repoPath string) error { return nil }
func (g *mockGitEngine) OperationAbort(ctx context.Context, repoPath string) error    { return nil }
func (g *mockGitEngine) WorktreeAdd(ctx context.Context, repoPath, worktreePath, branch string) error {
	return nil
}

func (g *mockGitEngine) WorktreeRemove(ctx context.Context, repoPath, worktreePath string) error {
	return nil
}

func (g *mockGitEngine) WorktreeList(ctx context.Context, repoPath string) ([]gitengine.WorktreeEntry, error) {
	return nil, nil
}

func (g *mockGitEngine) RebaseOnto(ctx context.Context, repoPath, newTip, forkPoint, branch string) error {
	return nil
}

func (g *mockGitEngine) MergeFFOnly(ctx context.Context, repoPath, branch string) error { return nil }

func (g *mockGitEngine) RebaseThenFFMerge(
	ctx context.Context,
	childWorktree, parentBranch, parentWorktree, childBranch string,
) error {
	return nil
}

func (g *mockGitEngine) WorkingTreeSummary(ctx context.Context, repoPath, forkPointSha string) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

func (g *mockGitEngine) MergeBase(ctx context.Context, repoPath, a, b string) (string, error) {
	return "", nil
}

func (g *mockGitEngine) WorktreeAddBranch(ctx context.Context, repoPath, worktreePath, branch, startPoint string) (string, error) {
	return "", nil
}

func (g *mockGitEngine) RevParse(ctx context.Context, repoPath, rev string) (string, error) {
	return "", nil
}

func (g *mockGitEngine) MergeSquash(ctx context.Context, repoPath, branch, subject string) error {
	return nil
}

var _ gitengine.Engine = (*mockGitEngine)(nil)

// --- helpers ---

func fixedNow() time.Time {
	return time.Unix(1_000_000, 0).UTC()
}

func newTestUsecase(
	ws *mockWorkspace,
	threads *mockReviewThread,
	chats *mockChat,
	repoStore store.Store[domain.Repository, string],
	gitEng *mockGitEngine,
) usecases.BranchReviewUsecase {
	return usecases.NewBranchReviewUsecase(
		ws,
		threads,
		chats,
		repoStore,
		gitEng,
		fixedNow,
	)
}

func noopThreads() *mockReviewThread {
	return &mockReviewThread{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.ReviewThread, error) {
			return nil, nil
		},
	}
}

func noopChats() *mockChat {
	return &mockChat{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.Chat, error) {
			return nil, nil
		},
	}
}

// --- Task 4: BranchReviewUsecase.Get tests ---

func TestBranchReview_Get_RootUsesDefaultBranch(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{
		ID:            "ws1",
		RepoID:        "repo1",
		Branch:        "feature",
		WorktreePath:  "/wt/feature",
		MergeStrategy: gitdomain.MergeStrategyMerge,
	}
	repo := domain.Repository{ID: "repo1", DefaultBranch: "main"}
	expectedDiff := gitdomain.MultiFileDiff{Files: []gitdomain.FileDiff{{FilePath: "foo.go"}}}
	expectedThreads := []domain.ReviewThread{{ID: "t1"}}

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "ws1" {
				return ws, nil
			}
			return domain.Workspace{}, errors.New("not found")
		},
	}
	repoStore := mocks.NewRepositoryStore()
	_ = repoStore.Save(ctx, repo)

	threads := &mockReviewThread{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.ReviewThread, error) {
			return expectedThreads, nil
		},
	}
	chats := &mockChat{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.Chat, error) {
			return nil, nil
		},
	}
	var gotBase, gotBranch string
	gitEng := &mockGitEngine{
		RangeDiffFn: func(_ context.Context, _, base, branch string) (gitdomain.MultiFileDiff, error) {
			gotBase = base
			gotBranch = branch
			return expectedDiff, nil
		},
	}

	uc := newTestUsecase(wsMock, threads, chats, repoStore, gitEng)

	review, err := uc.Get(ctx, "ws1")

	require.NoError(t, err)
	assert.Equal(t, "main", gotBase)
	assert.Equal(t, "feature", gotBranch)
	assert.Equal(t, expectedDiff, review.Diff)
	assert.Equal(t, expectedThreads, review.Threads)
	assert.Equal(t, gitdomain.MergeStrategyMerge, review.MergeStrategy)
	assert.Empty(t, review.Description)
}

func TestBranchReview_Get_ChildUsesParentBranch(t *testing.T) {
	ctx := context.Background()

	child := domain.Workspace{
		ID:           "child",
		RepoID:       "repo1",
		Branch:       "feat/child",
		WorktreePath: "/wt/child",
		ParentID:     "parent",
	}
	parent := domain.Workspace{
		ID:     "parent",
		Branch: "develop",
	}

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			switch id {
			case "child":
				return child, nil
			case "parent":
				return parent, nil
			}
			return domain.Workspace{}, errors.New("not found")
		},
	}
	repoStore := mocks.NewRepositoryStore()
	var gotBase string
	gitEng := &mockGitEngine{
		RangeDiffFn: func(_ context.Context, _, base, _ string) (gitdomain.MultiFileDiff, error) {
			gotBase = base
			return gitdomain.MultiFileDiff{}, nil
		},
	}

	uc := newTestUsecase(wsMock, noopThreads(), noopChats(), repoStore, gitEng)

	_, err := uc.Get(ctx, "child")

	require.NoError(t, err)
	assert.Equal(t, "develop", gotBase)
}

func TestBranchReview_Get_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errors.New("workspace: get: not found")
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Get(ctx, "missing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: get workspace")
}

func TestBranchReview_Get_RepoNil(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "gone", Branch: "feat", WorktreePath: "/wt"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore() // empty — FindByKey returns nil, nil

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, repoStore, &mockGitEngine{})

	_, err := uc.Get(ctx, "ws1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve base")
}

func TestBranchReview_Get_RepoStoreError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "r1", Branch: "feat", WorktreePath: "/wt"}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	// Use a store that errors on FindByKey
	repoStore := &errRepoStore{err: errors.New("db: connection lost")}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, repoStore, &mockGitEngine{})

	_, err := uc.Get(ctx, "ws1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve base")
}

func TestBranchReview_Get_DiffError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "r1", Branch: "feat", WorktreePath: "/wt"}
	repo := domain.Repository{ID: "r1", DefaultBranch: "main"}

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()
	_ = repoStore.Save(ctx, repo)

	gitEng := &mockGitEngine{
		RangeDiffFn: func(_ context.Context, _, _, _ string) (gitdomain.MultiFileDiff, error) {
			return gitdomain.MultiFileDiff{}, errors.New("git: diff failed")
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, repoStore, gitEng)

	_, err := uc.Get(ctx, "ws1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: diff")
}

func TestBranchReview_Get_ThreadsError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "r1", Branch: "feat", WorktreePath: "/wt"}
	repo := domain.Repository{ID: "r1", DefaultBranch: "main"}

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()
	_ = repoStore.Save(ctx, repo)

	threads := &mockReviewThread{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.ReviewThread, error) {
			return nil, errors.New("threads: db error")
		},
	}
	gitEng := &mockGitEngine{
		RangeDiffFn: func(_ context.Context, _, _, _ string) (gitdomain.MultiFileDiff, error) {
			return gitdomain.MultiFileDiff{}, nil
		},
	}

	uc := newTestUsecase(wsMock, threads, &mockChat{}, repoStore, gitEng)

	_, err := uc.Get(ctx, "ws1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: threads")
}

func TestBranchReview_Get_ChatsError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "r1", Branch: "feat", WorktreePath: "/wt"}
	repo := domain.Repository{ID: "r1", DefaultBranch: "main"}

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) { return ws, nil },
	}
	repoStore := mocks.NewRepositoryStore()
	_ = repoStore.Save(ctx, repo)

	threads := &mockReviewThread{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.ReviewThread, error) {
			return nil, nil
		},
	}
	chats := &mockChat{
		ListByWorkspaceFn: func(_ context.Context, _ string) ([]domain.Chat, error) {
			return nil, errors.New("chats: db error")
		},
	}
	gitEng := &mockGitEngine{
		RangeDiffFn: func(_ context.Context, _, _, _ string) (gitdomain.MultiFileDiff, error) {
			return gitdomain.MultiFileDiff{}, nil
		},
	}

	uc := newTestUsecase(wsMock, threads, chats, repoStore, gitEng)

	_, err := uc.Get(ctx, "ws1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: chats")
}

// --- Task 5: mutation tests ---

func TestBranchReview_SetMergeStrategy_Delegates(t *testing.T) {
	ctx := context.Background()

	var gotID string
	var gotStrategy gitdomain.MergeStrategy
	wsMock := &mockWorkspace{
		SetMergeStrategyFn: func(_ context.Context, id string, s gitdomain.MergeStrategy) (domain.Workspace, error) {
			gotID = id
			gotStrategy = s
			return domain.Workspace{ID: id, MergeStrategy: s}, nil
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	err := uc.SetMergeStrategy(ctx, "ws1", gitdomain.MergeStrategySquash)

	require.NoError(t, err)
	assert.Equal(t, "ws1", gotID)
	assert.Equal(t, gitdomain.MergeStrategySquash, gotStrategy)
}

func TestBranchReview_SetMergeStrategy_Error(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		SetMergeStrategyFn: func(_ context.Context, _ string, _ gitdomain.MergeStrategy) (domain.Workspace, error) {
			return domain.Workspace{}, errors.New("ws: set failed")
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	err := uc.SetMergeStrategy(ctx, "ws1", gitdomain.MergeStrategyMerge)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: set merge strategy")
}

func TestBranchReview_OpenThread_MintsIDsAndOpens(t *testing.T) {
	ctx := context.Background()

	var capturedInput reviewthread.OpenInput
	threads := &mockReviewThread{
		OpenFn: func(_ context.Context, in reviewthread.OpenInput, _ time.Time) (domain.ReviewThread, error) {
			capturedInput = in
			return domain.ReviewThread{ID: in.ID}, nil
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	in := usecases.OpenThreadInput{
		WsID:       "ws1",
		FilePath:   "pkg/foo.go",
		LineNumber: 42,
		Side:       domain.ReviewSide("right"),
		Body:       "nit: rename this",
	}
	got, err := uc.OpenThread(ctx, in)

	require.NoError(t, err)
	assert.NotEmpty(t, capturedInput.ID)
	assert.NotEmpty(t, capturedInput.MessageID)
	assert.NotEqual(t, capturedInput.ID, capturedInput.MessageID, "thread ID and message ID must be distinct UUIDs")
	assert.Equal(t, "ws1", capturedInput.WsID)
	assert.Equal(t, "pkg/foo.go", capturedInput.FilePath)
	assert.Equal(t, 42, capturedInput.LineNumber)
	assert.Equal(t, "nit: rename this", capturedInput.Body)
	assert.NotEmpty(t, got.ID)
}

func TestBranchReview_OpenThread_Error(t *testing.T) {
	ctx := context.Background()

	threads := &mockReviewThread{
		OpenFn: func(_ context.Context, _ reviewthread.OpenInput, _ time.Time) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errors.New("asynx: handler error")
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.OpenThread(ctx, usecases.OpenThreadInput{WsID: "ws1", Body: "x"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: open thread")
}

func TestBranchReview_Reply_MintsMessageID(t *testing.T) {
	ctx := context.Background()

	var capturedMsgID string
	threads := &mockReviewThread{
		ReplyFn: func(_ context.Context, id, messageID, body string, _ time.Time) (domain.ReviewThread, error) {
			capturedMsgID = messageID
			return domain.ReviewThread{ID: id}, nil
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Reply(ctx, "t1", "looks good")

	require.NoError(t, err)
	assert.NotEmpty(t, capturedMsgID)
}

func TestBranchReview_Reply_Error(t *testing.T) {
	ctx := context.Background()

	threads := &mockReviewThread{
		ReplyFn: func(_ context.Context, _, _, _ string, _ time.Time) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errors.New("not found")
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Reply(ctx, "t1", "body")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: reply")
}

func TestBranchReview_SetThreadResolved_TrueResolves(t *testing.T) {
	ctx := context.Background()

	var resolvedID string
	threads := &mockReviewThread{
		ResolveFn: func(_ context.Context, id string) (domain.ReviewThread, error) {
			resolvedID = id
			return domain.ReviewThread{ID: id}, nil
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	got, err := uc.SetThreadResolved(ctx, "t1", true)

	require.NoError(t, err)
	assert.Equal(t, "t1", resolvedID)
	assert.Equal(t, "t1", got.ID)
}

func TestBranchReview_SetThreadResolved_FalseReopens(t *testing.T) {
	ctx := context.Background()

	var reopenedID string
	threads := &mockReviewThread{
		ReopenFn: func(_ context.Context, id string) (domain.ReviewThread, error) {
			reopenedID = id
			return domain.ReviewThread{ID: id}, nil
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	got, err := uc.SetThreadResolved(ctx, "t2", false)

	require.NoError(t, err)
	assert.Equal(t, "t2", reopenedID)
	assert.Equal(t, "t2", got.ID)
}

func TestBranchReview_SetThreadResolved_ResolveError(t *testing.T) {
	ctx := context.Background()

	threads := &mockReviewThread{
		ResolveFn: func(_ context.Context, _ string) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errors.New("resolve: failed")
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.SetThreadResolved(ctx, "t1", true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve")
}

func TestBranchReview_SetThreadResolved_ReopenError(t *testing.T) {
	ctx := context.Background()

	threads := &mockReviewThread{
		ReopenFn: func(_ context.Context, _ string) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errors.New("reopen: failed")
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.SetThreadResolved(ctx, "t1", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: reopen")
}

func TestBranchReview_Get_ParentGetError(t *testing.T) {
	ctx := context.Background()

	child := domain.Workspace{
		ID:           "child",
		RepoID:       "r1",
		Branch:       "feat",
		WorktreePath: "/wt",
		ParentID:     "parent",
	}
	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, id string) (domain.Workspace, error) {
			if id == "child" {
				return child, nil
			}
			return domain.Workspace{}, errors.New("parent: not found")
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, &mockChat{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Get(ctx, "child")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: resolve base")
}

// errRepoStore is a store.Store[domain.Repository, string] that always errors on FindByKey.
type errRepoStore struct {
	err error
}

func (s *errRepoStore) Save(_ context.Context, _ domain.Repository) error { return nil }
func (s *errRepoStore) Delete(_ context.Context, _ string) error          { return nil }
func (s *errRepoStore) FindByKey(_ context.Context, _ string) (*domain.Repository, error) {
	return nil, s.err
}
func (s *errRepoStore) FindAll(_ context.Context) ([]domain.Repository, error) { return nil, nil }
