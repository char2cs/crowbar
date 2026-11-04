package branchreview_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestBranchReview_Get_MissingWorkspace_IsNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, asynxModels.ErrNotFound
		},
	}
	uc := newTestUsecase(wsMock, &mockReviewThread{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Get(ctx, "nope")
	require.ErrorIs(t, err, problem.ErrNotFound)
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

func (m *mockWorkspace) ObserveWorkingTreeState(ctx context.Context, in workspace.SyncInput, now time.Time) (domain.Workspace, error) {
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

func (m *mockWorkspace) ResolveConflicts(ctx context.Context, id string, now time.Time) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) UpdateForkPoint(ctx context.Context, id, forkPointSha string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) ProvisionInPlace(
	_ context.Context,
	id string,
	worktreePath string,
	forkPointSha string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: worktreePath, ForkPointSha: forkPointSha, Provisioning: domain.WorkspaceProvisioned}, nil
}

func (m *mockWorkspace) ClearBranch(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id}, nil
}

func (m *mockWorkspace) Relocate(
	_ context.Context,
	id string,
	worktreePath string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, WorktreePath: worktreePath, Provisioning: domain.WorkspaceProvisioned}, nil
}

func (m *mockWorkspace) RenameBranch(
	_ context.Context,
	id string,
	branch string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, Branch: branch}, nil
}

func (m *mockWorkspace) Delete(ctx context.Context, id string) error { return nil }
func (m *mockWorkspace) List(ctx context.Context) ([]domain.Workspace, error) {
	return nil, nil
}

func (m *mockWorkspace) SetParentFromPR(ctx context.Context, id, parentID string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) SetProject(_ context.Context, id, projectID string) (domain.Workspace, error) {
	return domain.Workspace{ID: id, ProjectID: projectID}, nil
}

func (m *mockWorkspace) GetHomeForProject(_ context.Context, _ string) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func (m *mockWorkspace) ListInRepo(_ context.Context, _, _ string) ([]domain.Workspace, error) {
	return nil, nil
}

var _ workspace.Workspace = (*mockWorkspace)(nil)

// --- local reviewthread mock ---

type mockReviewThread struct {
	OpenFn            func(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error)
	ReplyFn           func(ctx context.Context, in reviewthread.ReplyInput, now time.Time) (domain.ReviewThread, error)
	ResolveFn         func(ctx context.Context, id string) (domain.ReviewThread, error)
	ReopenFn          func(ctx context.Context, id string) (domain.ReviewThread, error)
	ListByWorkspaceFn func(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
}

func (m *mockReviewThread) Open(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error) {
	return m.OpenFn(ctx, in, now)
}

func (m *mockReviewThread) Reply(ctx context.Context, in reviewthread.ReplyInput, now time.Time) (domain.ReviewThread, error) {
	return m.ReplyFn(ctx, in, now)
}

func (m *mockReviewThread) EditMessage(_ context.Context, _, _, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (m *mockReviewThread) DeleteMessage(_ context.Context, _, _ string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (m *mockReviewThread) DeleteThread(_ context.Context, _ string) error { return nil }

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

// mockGitEngine fakes what branchreview calls; the rest of the engine is nil.
type mockGitEngine struct {
	gitengine.Engine  // the write verbs, which a review never runs
	RangeDiffFn       func(ctx context.Context, repoPath, base, branch string) (gitdomain.MultiFileDiff, error)
	DiffAgainstRefFn  func(ctx context.Context, repoPath, ref string) (gitdomain.MultiFileDiff, error)
	ReviewFilesFn     func(ctx context.Context, repoPath, ref string, dirty []string) ([]gitdomain.ReviewFileSummary, error)
	ReviewFilePatchFn func(ctx context.Context, repoPath, ref, path string, maxLines int, w io.Writer) (int, bool, error)
	ReviewFileSidesFn func(ctx context.Context, repoPath, ref, path, oldPath string) (gitdomain.ReviewFileSides, error)
	ReviewOutlineFn   func(ctx context.Context, repoPath, ref string) ([]gitdomain.FileOutline, error)
	ReviewSearchFn    func(ctx context.Context, repoPath, ref, query string, opts gitdomain.SearchOpts) ([]gitdomain.SearchHit, bool, error)
	MergeBaseFn       func(ctx context.Context, repoPath, a, b string) (string, error)
	StatusFn          func(ctx context.Context, repoPath string) (gitdomain.GitStatus, error)
	RevParseFn        func(ctx context.Context, repoPath, rev string) (string, error)
}

func (g *mockGitEngine) RangeDiff(ctx context.Context, repoPath, base, branch string) (gitdomain.MultiFileDiff, error) {
	if g.RangeDiffFn != nil {
		return g.RangeDiffFn(ctx, repoPath, base, branch)
	}
	return gitdomain.MultiFileDiff{}, nil
}

func (g *mockGitEngine) DiffAgainstRef(ctx context.Context, repoPath, ref string) (gitdomain.MultiFileDiff, error) {
	if g.DiffAgainstRefFn != nil {
		return g.DiffAgainstRefFn(ctx, repoPath, ref)
	}
	return gitdomain.MultiFileDiff{}, nil
}

func (g *mockGitEngine) ReviewFiles(
	ctx context.Context,
	repoPath, ref string,
	dirty []string,
) ([]gitdomain.ReviewFileSummary, error) {
	if g.ReviewFilesFn != nil {
		return g.ReviewFilesFn(ctx, repoPath, ref, dirty)
	}
	return nil, nil
}

func (g *mockGitEngine) ReviewFilePatch(
	ctx context.Context,
	repoPath, ref, path string,
	maxLines int,
	w io.Writer,
) (int, bool, error) {
	if g.ReviewFilePatchFn != nil {
		return g.ReviewFilePatchFn(ctx, repoPath, ref, path, maxLines, w)
	}
	return 0, false, nil
}

func (g *mockGitEngine) ReviewFileSides(
	ctx context.Context,
	repoPath, ref, path, oldPath string,
) (gitdomain.ReviewFileSides, error) {
	if g.ReviewFileSidesFn != nil {
		return g.ReviewFileSidesFn(ctx, repoPath, ref, path, oldPath)
	}
	return gitdomain.ReviewFileSides{}, nil
}

func (g *mockGitEngine) ReviewOutline(
	ctx context.Context,
	repoPath, ref string,
) ([]gitdomain.FileOutline, error) {
	if g.ReviewOutlineFn != nil {
		return g.ReviewOutlineFn(ctx, repoPath, ref)
	}
	return nil, nil
}

func (g *mockGitEngine) ReviewSearch(
	ctx context.Context,
	repoPath, ref, query string,
	opts gitdomain.SearchOpts,
) ([]gitdomain.SearchHit, bool, error) {
	if g.ReviewSearchFn != nil {
		return g.ReviewSearchFn(ctx, repoPath, ref, query, opts)
	}
	return nil, false, nil
}

func (g *mockGitEngine) Status(ctx context.Context, repoPath string) (gitdomain.GitStatus, error) {
	if g.StatusFn != nil {
		return g.StatusFn(ctx, repoPath)
	}
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

func (g *mockGitEngine) StageHunk(context.Context, string, string, string) error { return nil }

func (g *mockGitEngine) UnstageFile(ctx context.Context, repoPath, filePath string) error { return nil }

func (g *mockGitEngine) UnstageHunk(context.Context, string, string, string) error { return nil }

func (g *mockGitEngine) Discard(ctx context.Context, repoPath, filePath string) error { return nil }

func (g *mockGitEngine) Commit(ctx context.Context, repoPath, subject, body string) error { return nil }

func (g *mockGitEngine) Push(ctx context.Context, repoPath string) error { return nil }

func (g *mockGitEngine) Fetch(ctx context.Context, repoPath string) error { return nil }

func (g *mockGitEngine) FetchRef(ctx context.Context, repoPath, branch string) error { return nil }

func (g *mockGitEngine) FetchPrune(ctx context.Context, repoPath string) error { return nil }

func (g *mockGitEngine) FastForwardBranch(ctx context.Context, repoPath, branch string) error {
	return nil
}

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
func (g *mockGitEngine) OperationInProgress(ctx context.Context, repoPath string) (string, error) {
	return "", nil
}

func (g *mockGitEngine) WorktreeAdd(ctx context.Context, repoPath, worktreePath, branch string) error {
	return nil
}

func (g *mockGitEngine) WorktreeRemove(ctx context.Context, repoPath, worktreePath string, _ bool) error {
	return nil
}

func (g *mockGitEngine) UncommittedFiles(context.Context, string) (int, error) {
	return 0, nil
}

func (g *mockGitEngine) UnmergedCommits(context.Context, string, []string, string) (int, error) {
	return 0, nil
}

func (g *mockGitEngine) WorktreeRepair(ctx context.Context, repoPath, worktreePath string) error {
	return nil
}

func (g *mockGitEngine) WorktreePrune(ctx context.Context, repoPath string) error {
	return nil
}

func (g *mockGitEngine) WorktreeList(ctx context.Context, repoPath string) ([]gitengine.WorktreeEntry, error) {
	return nil, nil
}

func (g *mockGitEngine) DetachWorktree(ctx context.Context, worktreePath string) error { return nil }

func (g *mockGitEngine) CheckoutBranch(ctx context.Context, worktreePath, branch string) error {
	return nil
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

func (g *mockGitEngine) ResolveDiffBase(context.Context, string, string) gitdomain.DiffBase {
	return gitdomain.DiffBase{}
}

func (g *mockGitEngine) DiffStat(context.Context, string, string) (int, int, error) {
	return 0, 0, nil
}

func (g *mockGitEngine) WorkingTreeSummary(ctx context.Context, repoPath, forkPointSha string) (int, int, bool, bool, error) {
	return 0, 0, false, false, nil
}

func (g *mockGitEngine) WouldMergeConflicts(
	ctx context.Context, repoPath string, pairs []gitengine.MergePair,
) []gitengine.MergeVerdict {
	return make([]gitengine.MergeVerdict, len(pairs))
}

func (g *mockGitEngine) MergeBase(ctx context.Context, repoPath, a, b string) (string, error) {
	if g.MergeBaseFn != nil {
		return g.MergeBaseFn(ctx, repoPath, a, b)
	}
	return a, nil
}

func (g *mockGitEngine) WorktreeAddBranch(ctx context.Context, repoPath, worktreePath, branch, startPoint string) (string, error) {
	return "", nil
}

func (g *mockGitEngine) WorktreeAddAtRef(ctx context.Context, repoPath, worktreePath, branch, startRef string) (string, error) {
	return "", nil
}

func (g *mockGitEngine) RevParse(ctx context.Context, repoPath, rev string) (string, error) {
	if g.RevParseFn != nil {
		return g.RevParseFn(ctx, repoPath, rev)
	}
	return "", nil
}

func (g *mockGitEngine) MergeSquash(ctx context.Context, repoPath, branch, subject string) error {
	return nil
}

func (g *mockGitEngine) RemoteBranchExists(ctx context.Context, repoPath, branch string) (bool, error) {
	return false, nil
}

func (g *mockGitEngine) RemoteTrackingBranchExists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (g *mockGitEngine) SetUpstream(ctx context.Context, repoPath, branch string) error {
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
	repoStore store.Store[domain.Repository, string],
	gitEng *mockGitEngine,
) branchreview.Usecase {
	return branchreview.New(
		ws,
		threads,
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

// --- Task 4: BranchReviewUsecase.Get tests ---

func TestBranchReview_Get_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()

	wsMock := &mockWorkspace{
		GetFn: func(_ context.Context, _ string) (domain.Workspace, error) {
			return domain.Workspace{}, errors.New("workspace: get: not found")
		},
	}

	uc := newTestUsecase(wsMock, &mockReviewThread{}, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Get(ctx, "missing")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: get workspace")
}

func TestBranchReview_Get_ThreadsError(t *testing.T) {
	ctx := context.Background()

	ws := domain.Workspace{ID: "ws1", RepoID: "r1", Branch: "feat", WorktreePath: "/wt", Provisioning: domain.WorkspaceProvisioned}
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

	uc := newTestUsecase(wsMock, threads, repoStore, gitEng)

	_, err := uc.Get(ctx, "ws1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: threads")
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

	uc := newTestUsecase(wsMock, &mockReviewThread{}, mocks.NewRepositoryStore(), &mockGitEngine{})

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

	uc := newTestUsecase(wsMock, &mockReviewThread{}, mocks.NewRepositoryStore(), &mockGitEngine{})

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

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

	in := branchreview.OpenThreadInput{
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

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.OpenThread(ctx, branchreview.OpenThreadInput{WsID: "ws1", Body: "x"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: open thread")
}

func TestBranchReview_Reply_MintsMessageID(t *testing.T) {
	ctx := context.Background()

	var captured reviewthread.ReplyInput
	threads := &mockReviewThread{
		ReplyFn: func(_ context.Context, in reviewthread.ReplyInput, _ time.Time) (domain.ReviewThread, error) {
			captured = in
			return domain.ReviewThread{ID: in.ID}, nil
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.Reply(ctx, "t1", "looks good")

	require.NoError(t, err)
	assert.NotEmpty(t, captured.MessageID)
	// The human/UI path attributes nothing: a person has neither a vendor CLI
	// behind them nor an originating agent conversation.
	assert.Empty(t, captured.ProviderID)
	assert.Empty(t, captured.ChatID)
	assert.False(t, captured.IsAgent)
}

func TestBranchReview_Reply_Error(t *testing.T) {
	ctx := context.Background()

	threads := &mockReviewThread{
		ReplyFn: func(_ context.Context, _ reviewthread.ReplyInput, _ time.Time) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, errors.New("not found")
		},
	}

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

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

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

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

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

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

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

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

	uc := newTestUsecase(&mockWorkspace{}, threads, mocks.NewRepositoryStore(), &mockGitEngine{})

	_, err := uc.SetThreadResolved(ctx, "t1", false)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "branch review: reopen")
}

// --- ForkPointSha fallback and annotateUncommitted tests ---

func (m *mockWorkspace) SetLock(
	_ context.Context,
	id string,
	locked *bool,
	_ bool,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id, LockOverride: locked}, nil
}
