package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

type fakeWorktreeReads struct {
	rows        []domain.Workspace
	listCalls   int
	getErr      error
	listErr     error
	elig        workspace.MergeEligibility
	eligCalls   atomic.Int32
	eligBatches atomic.Int32
}

func (f *fakeWorktreeReads) Get(
	_ context.Context,
	workspaceID string,
) (domain.Workspace, error) {
	if f.getErr != nil {
		return domain.Workspace{}, f.getErr
	}
	for _, w := range f.rows {
		if w.ID == workspaceID {
			return w, nil
		}
	}
	return domain.Workspace{}, errors.New("no such workspace")
}

func (f *fakeWorktreeReads) ListInRepo(
	_ context.Context,
	_ string,
	_ string,
) ([]domain.Workspace, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.rows, nil
}

func (f *fakeWorktreeReads) MergeEligibilitiesFor(
	_ context.Context,
	wss []domain.Workspace,
	_ []domain.Workspace,
) []workspace.MergeEligibility {
	f.eligBatches.Add(1)
	f.eligCalls.Add(int32(len(wss)))
	out := make([]workspace.MergeEligibility, len(wss))
	for i := range out {
		out[i] = f.elig
	}
	return out
}

func newWorktreeHandlers(
	uc *configurableListGetUsecase,
	worktrees handlers.Worktrees,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, &fakeChatTree{}).WithWorktrees(worktrees)
}

type fakeNodeReads struct {
	rows map[string]tree.Node
}

func (f *fakeNodeReads) GetNode(
	_ context.Context,
	id string,
) (tree.Node, error) {
	n, ok := f.rows[id]
	if !ok {
		return tree.Node{}, errors.New("no such node")
	}
	return n, nil
}

func TestRegression_List_AnOrdinaryForksPlacementReadsItsOwningChatsOwnFields(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{
		{ID: "fork-chat", WorkspaceID: "ws-fork", ParentID: "branch-1", Order: 3, OwnsWorkspace: true},
	}}
	worktrees := &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-fork", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", Status: domain.WorkspaceStatusNew},
	}}
	nodes := &fakeNodeReads{rows: map[string]tree.Node{
		"ws-fork": {ID: "ws-fork", ParentID: "", Order: 0},
	}}
	h := newWorktreeHandlers(uc, worktrees).WithNodes(nodes)

	rows := listChats(t, h)

	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Worktree)
	assert.Equal(t, "branch-1", rows[0].Worktree.FolderID, "must read the chat's own placement, not its abandoned Node")
	assert.Equal(t, 3, rows[0].Worktree.Order)
}

func TestRegression_List_ALockedBranchStillReadsItsOwnWorkspaceAnchorNode(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{
		{ID: "owner-chat", WorkspaceID: "ws-locked"},
	}}
	worktrees := &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-locked", RepoID: "r1", ProjectID: "p1", Branch: "main", Status: domain.WorkspaceStatusLocked},
	}}
	nodes := &fakeNodeReads{rows: map[string]tree.Node{
		"ws-locked": {ID: "ws-locked", ParentID: "folder-1", Order: 2},
	}}
	h := newWorktreeHandlers(uc, worktrees).WithNodes(nodes)

	rows := listChats(t, h)

	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Worktree)
	assert.Equal(t, "folder-1", rows[0].Worktree.FolderID)
	assert.Equal(t, 2, rows[0].Worktree.Order)
}

func listChats(
	t *testing.T,
	h *handlers.Handlers,
) []dto.AgentChatDTO {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats", nil)
	ctx.Params = gin.Params{{Key: "projectId", Value: "p1"}, {Key: "repoId", Value: "r1"}}

	h.List(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var envelope struct {
		Data []dto.AgentChatDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func TestList_AWorktreeOwningChatCarriesItsGitState(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws-1"}}}
	worktrees := &fakeWorktreeReads{
		rows: []domain.Workspace{{
			ID: "ws-1", RepoID: "r1", ProjectID: "p1",
			Branch: "feature/x", Added: 5, Deleted: 2,
		}},
		elig: workspace.MergeEligibility{CanMergeLocally: true, ParentBranch: "main"},
	}

	rows := listChats(t, newWorktreeHandlers(uc, worktrees))

	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Worktree, "a chat that owns a worktree must carry it")
	assert.Equal(t, "feature/x", rows[0].Worktree.Branch)
	assert.Equal(t, 5, rows[0].Worktree.Added)
	assert.Equal(t, 2, rows[0].Worktree.Deleted)
	assert.True(t, rows[0].Worktree.CanMergeLocally, "the merge overlay rides it too")
	assert.Equal(t, "main", rows[0].Worktree.ParentBranch)
}

func TestList_ABubbleCarriesNoWorktree(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1"}}}

	rows := listChats(t, newWorktreeHandlers(uc, &fakeWorktreeReads{}))

	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].Worktree)
}

func TestList_EveryChatSharingAWorktreeNamesTheSameOwner(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{
		{ID: "owner", WorkspaceID: "ws-1", OwnsWorkspace: true},
		{ID: "thread", WorkspaceID: "ws-1", ParentID: "owner"},
	}}
	worktrees := &fakeWorktreeReads{
		rows: []domain.Workspace{{ID: "ws-1", RepoID: "r1", ProjectID: "p1", Branch: "feature/x"}},
	}

	rows := listChats(t, newWorktreeHandlers(uc, worktrees))

	require.Len(t, rows, 2)
	for _, row := range rows {
		require.NotNil(t, row.Worktree, "both rows read the same worktree")
		assert.Equal(t, "feature/x", row.Worktree.Branch)
		assert.Equal(t, "owner", row.Worktree.OwningChatID,
			"and both name the SAME owner, so a client can tell which row is the worktree")
	}
}

func TestList_ReadsTheRepoSiblingsOnceForTheWholeList(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{
		{ID: "c1", WorkspaceID: "ws-1"},
		{ID: "c2", WorkspaceID: "ws-2"},
		{ID: "c3", WorkspaceID: "ws-1"},
	}}
	worktrees := &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-1", RepoID: "r1", ProjectID: "p1", Branch: "a"},
		{ID: "ws-2", RepoID: "r1", ProjectID: "p1", Branch: "b"},
	}}

	rows := listChats(t, newWorktreeHandlers(uc, worktrees))

	require.Len(t, rows, 3)
	assert.Equal(t, 1, worktrees.listCalls, "one repo-wide read serves the whole list")
}

func TestList_AnUnreadableWorktreeStillListsTheChat(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws-1"}}}
	worktrees := &fakeWorktreeReads{listErr: errors.New("read failed")}

	rows := listChats(t, newWorktreeHandlers(uc, worktrees))

	require.Len(t, rows, 1)
	assert.Equal(t, "c1", rows[0].ID)
	assert.Nil(t, rows[0].Worktree)
}

func TestList_UnwiredWorktreeReadsDegradeToAbsent(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws-1"}}}

	rows := listChats(t, handlers.New(uc, uc, uc, uc, uc, &fakeChatTree{}))

	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].Worktree)
}

func TestGet_ByIdCarriesTheWorktreeToo(t *testing.T) {
	uc := &configurableListGetUsecase{
		chat:  domain.Chat{ID: "c1", WorkspaceID: "ws-1", OwnsWorkspace: true},
		chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws-1", OwnsWorkspace: true}},
	}
	worktrees := &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-1", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", Added: 9},
	}}

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/chats/c1", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c1"}}

	newWorktreeHandlers(uc, worktrees).Get(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var envelope struct {
		Data dto.AgentChatDetailDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.NotNil(t, envelope.Data.Worktree)
	assert.Equal(t, "feature/x", envelope.Data.Worktree.Branch)
	assert.Equal(t, 9, envelope.Data.Worktree.Added)
	assert.Equal(t, "c1", envelope.Data.Worktree.OwningChatID)
}

func TestRegression_List_WorkTreeReadsAreOncePerWorkspaceNotPerChat(t *testing.T) {
	const workspaces, perWorkspace = 3, 5
	var chats []domain.Chat
	var rows []domain.Workspace
	for w := 0; w < workspaces; w++ {
		wsID := fmt.Sprintf("ws-%d", w)
		rows = append(rows, domain.Workspace{
			ID: wsID, RepoID: "r1", ProjectID: "p1", ParentID: "ws-0",
			Branch: "feature/" + wsID, Status: domain.WorkspaceStatusNew,
		})
		for c := 0; c < perWorkspace; c++ {
			chats = append(chats, domain.Chat{ID: fmt.Sprintf("%s-chat-%d", wsID, c), WorkspaceID: wsID})
		}
	}
	uc := &configurableListGetUsecase{chats: chats}
	worktrees := &fakeWorktreeReads{rows: rows, elig: workspace.MergeEligibility{CanMergeLocally: true}}
	h := newWorktreeHandlers(uc, worktrees).WithNodes(&fakeNodeReads{})

	got := listChats(t, h)

	require.Len(t, got, workspaces*perWorkspace)
	for _, c := range got {
		require.NotNil(t, c.Worktree)
		assert.True(t, c.Worktree.CanMergeLocally)
	}
	assert.EqualValues(t, workspaces, worktrees.eligCalls.Load(), "one eligibility resolution per workspace")
	assert.EqualValues(t, 1, worktrees.eligBatches.Load(), "one batched git read for the whole list")
	assert.Empty(t, uc.listWsIDs, "the owner and placement come from the chats already listed, not a re-read of the store")
}
