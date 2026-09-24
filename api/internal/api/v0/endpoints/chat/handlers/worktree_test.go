package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The chat DTO's git half (spec §5): ONE read of the chat list has to answer
// everything the deleted workspace list answered, so these tests all ask the
// same question — does a chat that owns a worktree come back carrying it?

// fakeWorktreeReads is the Worktrees port: the workspace a chat owns, its repo
// siblings, and the merge overlay resolved over them.
type fakeWorktreeReads struct {
	rows []domain.Workspace
	// listCalls counts the repo-wide reads, which is how the amortisation
	// contract is proved: a list of many chats must not take one read per row.
	listCalls int
	getErr    error
	listErr   error
	elig      workspace.MergeEligibility
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

func (f *fakeWorktreeReads) MergeEligibilityFor(
	_ context.Context,
	_ domain.Workspace,
	_ []domain.Workspace,
) workspace.MergeEligibility {
	return f.elig
}

func newWorktreeHandlers(
	uc *configurableListGetUsecase,
	worktrees handlers.Worktrees,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, &fakeChatTree{}, nil).WithWorktrees(worktrees)
}

// fakeNodeReads is the Nodes port: one Node row keyed by id.
type fakeNodeReads struct {
	rows map[string]domain.Node
}

func (f *fakeNodeReads) GetNode(
	_ context.Context,
	id string,
) (domain.Node, error) {
	n, ok := f.rows[id]
	if !ok {
		return domain.Node{}, errors.New("no such node")
	}
	return n, nil
}

// TestRegression_List_AnOrdinaryForksPlacementReadsItsOwningChatsOwnFields
// pins the live bug behind a fork whose folder placement snapped back to the
// repo root moments after landing: PlaceWorkspace's write for an ordinary
// fork (RendersAsBranch false) lands on its OWNING CHAT's own
// ParentID/Order via Chats.SetPlacement/SetOrder (place_workspace.go's
// nodeID doc) — a real AgentChat aggregate field. No Node is ever minted for
// a plain chat, so a Placement reader that looks one up via the Node store
// keyed by that same chat id always misses and degrades to "" / 0,
// regardless of how long ago the drag landed. Caught live: a fork dragged
// into a folder showed it there for a moment (the PATCH response's own
// echoed placement), then reseeded straight back to the repo root on the
// very next read.
func TestRegression_List_AnOrdinaryForksPlacementReadsItsOwningChatsOwnFields(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{
		// The fork's owning chat, carrying the placement a real drag wrote —
		// its OWN ParentID/Order, never a Node row.
		{ID: "fork-chat", WorkspaceID: "ws-fork", ParentID: "branch-1", Order: 3, OwnsWorkspace: true},
	}}
	worktrees := &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-fork", RepoID: "r1", ProjectID: "p1", Branch: "feature/x", Status: domain.WorkspaceStatusNew},
	}}
	// The fork's own, never-touched workspace-anchor Node — a fixed row that
	// must NOT be what this read answers from.
	nodes := &fakeNodeReads{rows: map[string]domain.Node{
		"ws-fork": {ID: "ws-fork", ParentID: "", Order: 0},
	}}
	h := newWorktreeHandlers(uc, worktrees).WithNodes(nodes)

	rows := listChats(t, h)

	require.Len(t, rows, 1)
	require.NotNil(t, rows[0].Worktree)
	assert.Equal(t, "branch-1", rows[0].Worktree.FolderID, "must read the chat's own placement, not its abandoned Node")
	assert.Equal(t, 3, rows[0].Worktree.Order)
}

// TestRegression_List_ALockedBranchStillReadsItsOwnWorkspaceAnchorNode pins
// the other half of the same fix: a LOCKED branch's owning chat carries no
// Node of its own, so its placement must still come from ws.ID's own
// Node{Kind:workspace} row — the one case a chat-based read cannot answer.
func TestRegression_List_ALockedBranchStillReadsItsOwnWorkspaceAnchorNode(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{
		{ID: "owner-chat", WorkspaceID: "ws-locked"},
	}}
	worktrees := &fakeWorktreeReads{rows: []domain.Workspace{
		{ID: "ws-locked", RepoID: "r1", ProjectID: "p1", Branch: "main", Status: domain.WorkspaceStatusLocked},
	}}
	nodes := &fakeNodeReads{rows: map[string]domain.Node{
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

// TestList_AWorktreeOwningChatCarriesItsGitState is spec §5's whole promise on
// the list route: the branch, the diff counts and the merge overlay ride the
// chat, so a client draws a branch row from this read alone.
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

// A bubble owns nothing, and says so by absence rather than by a zero object —
// see ChatWorktreeDTO's own doc.
func TestList_ABubbleCarriesNoWorktree(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1"}}}

	rows := listChats(t, newWorktreeHandlers(uc, &fakeWorktreeReads{}))

	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].Worktree)
}

// TestList_EveryChatSharingAWorktreeNamesTheSameOwner is the fact the frontend
// dedupes on. A worktree is many-chats-to-one, so a thread carrying its
// parent's workspace id gets the same git state (law 5 — shared state is
// shared); what tells a client which row IS the worktree is that they all agree
// on owningChatId, resolved once by the daemon rather than re-derived per
// client.
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

// TestList_ReadsTheRepoSiblingsOnceForTheWholeList pins the amortisation
// WorkspaceDTOList's own eligFn contract exists for: resolving eligibility
// needs the row's siblings, and reading them per row would turn one list into
// one repo-wide read per chat.
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

// An unreadable workspace must not blank the panel: the caller asked for the
// chat list, and a chat whose git state cannot be resolved is still a chat
// worth listing. This mirrors resolveOwningChatID's own degradation.
func TestList_AnUnreadableWorktreeStillListsTheChat(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws-1"}}}
	worktrees := &fakeWorktreeReads{listErr: errors.New("read failed")}

	rows := listChats(t, newWorktreeHandlers(uc, worktrees))

	require.Len(t, rows, 1)
	assert.Equal(t, "c1", rows[0].ID)
	assert.Nil(t, rows[0].Worktree)
}

// Unwired, every chat serializes without a worktree rather than panicking —
// the honest shape for the project-home mount, which mounts these handlers
// without the workspace reads because its row has no repo and no git surface at
// all.
func TestList_UnwiredWorktreeReadsDegradeToAbsent(t *testing.T) {
	uc := &configurableListGetUsecase{chats: []domain.Chat{{ID: "c1", WorkspaceID: "ws-1"}}}

	rows := listChats(t, handlers.New(uc, uc, uc, uc, uc, &fakeChatTree{}, nil))

	require.Len(t, rows, 1)
	assert.Nil(t, rows[0].Worktree)
}

// TestGet_ByIdCarriesTheWorktreeToo keeps the detail route in step with the
// list: a client that opened one chat must not have to fetch the list to learn
// its branch.
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

// mintingTree is fakeChatTree with a mint that succeeds and an attach that
// fails — the shape a request context cancelled between the two (the client
// reloading mid-GET) leaves behind.
type mintingTree struct {
	fakeChatTree
	minted   int
	discards int
}

func (m *mintingTree) MintOwningChat(
	_ context.Context,
	_ string,
) (string, error) {
	m.minted++
	return "minted-1", nil
}

func (m *mintingTree) AttachOwningWorkspace(
	_ context.Context,
	_ string,
	_ domain.Workspace,
) error {
	return errors.New("context canceled")
}

func (m *mintingTree) DiscardOwningChat(
	_ context.Context,
	_ string,
) error {
	m.discards++
	return nil
}
