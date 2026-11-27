package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain/tree"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

type fakeChatTree struct {
	createdWorkspace string
	list             []domain.Chat
	created          domain.Chat
	renamed          domain.Chat
	moved            domain.Chat
	placed           domain.Chat
	shifted          []domain.Chat
	deletion         agentusecase.ChatDeletion
	err              error

	gotCreate       agentusecase.CreateInput
	gotMove         agentusecase.MoveInput
	gotPlace        agentusecase.PlaceInput
	gotBranch       string
	gotRepoID       string
	gotRename       string
	gotID           string
	gotPlaceID      string
	gotCreate2      createChatCall
	gotPurge        string
	renames         int
	moves           int
	gotWorktree     agentusecase.WorktreeSpec
	gotSurface      string
	createChatCalls int
	gotHomeID       string
	folder          tree.Folder
}

func (f *fakeChatTree) ListInRepo(
	_ context.Context,
	repoID string,
) ([]domain.Chat, error) {
	f.gotRepoID = repoID
	return f.list, f.err
}

func (f *fakeChatTree) ListInHome(
	_ context.Context,
	homeID string,
) ([]domain.Chat, error) {
	f.gotHomeID = homeID
	return f.list, f.err
}

func (f *fakeChatTree) FolderScope(_ context.Context, id string) (tree.Folder, error) {
	if f.folder.ID == id {
		return f.folder, nil
	}
	return tree.Folder{ID: id, RepoID: "r1"}, nil
}

func (f *fakeChatTree) Create(
	_ context.Context,
	in agentusecase.CreateInput,
) (domain.Chat, []domain.Chat, error) {
	f.gotCreate = in
	return f.created, f.shifted, f.err
}

func (f *fakeChatTree) Rename(
	_ context.Context,
	id string,
	name string,
) (domain.Chat, error) {
	f.renames++
	f.gotID = id
	f.gotRename = name
	return f.renamed, f.err
}

func (f *fakeChatTree) Move(
	_ context.Context,
	id string,
	in agentusecase.MoveInput,
) (domain.Chat, []domain.Chat, error) {
	f.moves++
	f.gotID = id
	f.gotMove = in
	return f.moved, f.shifted, f.err
}

func (f *fakeChatTree) Delete(
	_ context.Context,
	id string,
) ([]domain.Chat, error) {
	f.gotID = id
	return f.shifted, f.err
}

type createChatCall struct {
	ID          string
	WorkspaceID string
	ProviderID  string
	ParentID    string
	OwnWorktree bool
}

func (f *fakeChatTree) CreateChat(
	_ context.Context,
	in agentusecase.CreateChatInput,
) (agentusecase.CreatedChat, error) {
	f.gotCreate2 = createChatCall{
		ID:          in.ID,
		WorkspaceID: in.WorkspaceID,
		ProviderID:  in.ProviderID,
		ParentID:    in.ParentID,
		OwnWorktree: in.Worktree.Mode == agentusecase.WorktreeFork,
	}
	f.gotWorktree = in.Worktree
	f.gotSurface = in.Surface
	f.createChatCalls++
	return agentusecase.CreatedChat{ChatID: f.placed.ID, RunnerID: "runner-1", WorkspaceID: f.createdWorkspace}, f.err
}

func (f *fakeChatTree) PlaceChat(
	_ context.Context,
	workspaceID string,
	chatID string,
	in agentusecase.PlaceInput,
) (domain.Chat, []domain.Chat, error) {
	f.gotID = workspaceID
	f.gotPlaceID = chatID
	f.gotPlace = in
	return f.placed, f.shifted, f.err
}

func (f *fakeChatTree) BranchOut(_ context.Context, chatID, branch string) error {
	f.gotPlaceID, f.gotBranch = chatID, branch
	return f.err
}

func (f *fakeChatTree) CancelMove(_ context.Context, chatID string) error {
	f.gotPlaceID = chatID
	return f.err
}

func (f *fakeChatTree) DeleteChat(
	_ context.Context,
	chatID string,
	_ domain.DeleteConsent,
) (agentusecase.ChatDeletion, error) {
	f.gotPurge = chatID
	return f.deletion, f.err
}

func (f *fakeChatTree) MintOwningChat(
	_ context.Context,
	_ string,
) (string, error) {
	return "", f.err
}

func (f *fakeChatTree) AttachOwningWorkspace(
	_ context.Context,
	_ string,
	_ domain.Workspace,
) error {
	return f.err
}

func (f *fakeChatTree) DiscardOwningChat(
	_ context.Context,
	_ string,
) error {
	return f.err
}

type agentUsecases interface {
	handlers.ChatUsecase
	handlers.TurnUsecase
	handlers.RunnerUsecase
	handlers.AnswerUsecase
	handlers.ProviderUsecase
}

func newChatHandlers(
	uc agentUsecases,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, &fakeChatTree{})
}

func newChatHandlersWith(
	uc agentUsecases,
	tree *fakeChatTree,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, tree)
}

func newFolderHandlers(
	tree *fakeChatTree,
) *handlers.Handlers {
	return newFolderHandlersWith(&fakeAgentUsecase{}, tree)
}

func newFolderHandlersWith(
	uc agentUsecases,
	tree *fakeChatTree,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, tree)
}

func folderParams(
	extra ...gin.Param,
) gin.Params {
	return append(gin.Params{{Key: "repoId", Value: "r1"}, {Key: "wsId", Value: "ws-1"}}, extra...)
}

func TestCreateFolder_TakesTheScopeFromTheURL(t *testing.T) {
	tree := &fakeChatTree{created: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Title: "spikes"}}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders",
		[]byte(`{"name":"spikes","parentId":"c1","repoId":"evil"}`))
	ctx.Params = folderParams()

	newFolderHandlers(tree).CreateFolder(ctx)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "r1", tree.gotCreate.RepoID)
	assert.Equal(t, "c1", tree.gotCreate.ParentID)
}

func TestCreateFolder_ReturnsTheCollateral(t *testing.T) {
	tree := &fakeChatTree{
		created: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder},
		shifted: []domain.Chat{{ID: "f0", Type: domain.ChatTypeFolder, Order: 0}},
	}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{"name":"spikes"}`))
	ctx.Params = folderParams()

	newFolderHandlers(tree).CreateFolder(ctx)

	require.Equal(t, http.StatusCreated, rec.Code)
	body := decodeFolderResponse(t, rec.Body.Bytes())
	assert.Equal(t, "f1", body.Folder.ID)
	require.Len(t, body.Shifted, 1)
	assert.Equal(t, "f0", body.Shifted[0].ID)
}

func TestCreateFolder_SurfacesTheUsecaseRefusal(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeNameRequired}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{"name":" "}`))
	ctx.Params = folderParams()

	newFolderHandlers(tree).CreateFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateFolder_MalformedBodyIs400(t *testing.T) {
	tree := &fakeChatTree{}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{`))
	ctx.Params = folderParams()

	newFolderHandlers(tree).CreateFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, tree.gotCreate.RepoID, "a malformed body must not reach the usecase")
}

func TestListFolders_ScopesToTheURLRepo(t *testing.T) {
	tree := &fakeChatTree{list: []domain.Chat{
		{ID: "b", Type: domain.ChatTypeFolder, Order: 1},
		{ID: "a", Type: domain.ChatTypeFolder, Order: 0},
	}}
	ctx, rec := newTestContext(t, http.MethodGet, "/chats/folders", nil)
	ctx.Params = folderParams()

	newFolderHandlers(tree).ListFolders(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Data []dto.AgentChatDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 2)
	assert.Equal(t, "a", env.Data[0].ID, "the list handler serves panel order")
	assert.Equal(t, "r1", tree.gotRepoID)
}

func TestListFolders_SurfacesAStoreError(t *testing.T) {
	tree := &fakeChatTree{err: problem.ErrNotFound}
	ctx, rec := newTestContext(t, http.MethodGet, "/chats/folders", nil)
	ctx.Params = folderParams()

	newFolderHandlers(tree).ListFolders(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPatchFolder_RenameThenMove(t *testing.T) {
	tree := &fakeChatTree{moved: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Title: "new", Order: 2}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1",
		[]byte(`{"name":"new","parentId":"c1","order":2}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, tree.renames)
	assert.Equal(t, 1, tree.moves)
	assert.Equal(t, "new", tree.gotRename)
	require.NotNil(t, tree.gotMove.ParentID)
	assert.Equal(t, "c1", *tree.gotMove.ParentID)
	require.NotNil(t, tree.gotMove.Order)
	assert.Equal(t, 2, *tree.gotMove.Order)
}

func TestRegression_PatchFolder_NameOnlySkipsTheMove(t *testing.T) {
	tree := &fakeChatTree{renamed: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Title: "new"}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"name":"new"}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, tree.renames)
	assert.Zero(t, tree.moves, "a bare rename must never run Move's own guardNotWorking")
	assert.Equal(t, "new", tree.gotRename)
}

func TestPatchFolder_OrderOnlyLeavesTheParentNil(t *testing.T) {
	tree := &fakeChatTree{moved: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Order: 1}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"order":1}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, tree.renames)
	assert.Nil(t, tree.gotMove.ParentID)
	require.NotNil(t, tree.gotMove.Order)
	assert.Equal(t, 1, *tree.gotMove.Order)
}

func TestPatchFolder_EmptyParentMeansThePanelRoot(t *testing.T) {
	tree := &fakeChatTree{moved: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"parentId":""}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, tree.gotMove.ParentID)
	assert.Equal(t, "", *tree.gotMove.ParentID)
}

func TestPatchFolder_FailedRenameSkipsTheMove(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeNameRequired}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"name":" ","order":0}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, tree.renames)
	assert.Zero(t, tree.moves, "the placement must not run after a refused rename")
}

func TestPatchFolder_CycleIsAConflict(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeCycle}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"parentId":"f2"}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestPatchFolder_MalformedBodyIs400(t *testing.T) {
	tree := &fakeChatTree{}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).PatchFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, tree.moves)
}

func TestDeleteFolder_DeletesTheAddressedFolder(t *testing.T) {
	tree := &fakeChatTree{shifted: []domain.Chat{{ID: "child", Type: domain.ChatTypeFolder}}}
	ctx, rec := newTestContext(t, http.MethodDelete, "/chats/folders/f1", nil)
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).DeleteFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "f1", tree.gotID)
}

func TestDeleteFolder_NotFoundIs404(t *testing.T) {
	tree := &fakeChatTree{err: problem.ErrNotFound}
	ctx, rec := newTestContext(t, http.MethodDelete, "/chats/folders/f1", nil)
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree).DeleteFolder(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestPlaceChat_ForwardsTheWorkspaceAMoveRunsIn(t *testing.T) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "c2", WorkspaceID: "ws-1"}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement",
		[]byte(`{"parentId":"b2","workspaceId":"ws-b2","label":"b2"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c2"}}

	newFolderHandlers(tree).PlaceChat(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-b2", tree.gotPlace.WorkspaceID)
	assert.Equal(t, "b2", tree.gotPlace.Label)
}

func TestBranchOut_AcceptsTheRequestAndNamesTheBranch(t *testing.T) {
	tree := &fakeChatTree{}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/c2/branch-out", []byte(`{"branch":" fix-it "}`))
	ctx.Params = gin.Params{{Key: "id", Value: "c2"}}

	newFolderHandlers(tree).BranchOut(ctx)

	require.Equal(t, http.StatusAccepted, rec.Code)
	assert.Equal(t, "c2", tree.gotPlaceID)
	assert.Equal(t, "fix-it", tree.gotBranch)
}

func TestBranchOut_AHomeThreadIsRefusedWithTheReason(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrNoRepoToBranch}
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/c2/branch-out", nil)
	ctx.Params = gin.Params{{Key: "id", Value: "c2"}}

	newFolderHandlers(tree).BranchOut(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestPlaceChat_ForwardsTheRequestedLineage(t *testing.T) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "c2", WorkspaceID: "ws-1", ParentID: "c1", Order: 3}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement",
		[]byte(`{"parentId":"c1","order":3}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c2"}}

	newFolderHandlers(tree).PlaceChat(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "c2", tree.gotPlaceID)
	require.NotNil(t, tree.gotPlace.ParentID)
	assert.Equal(t, "c1", *tree.gotPlace.ParentID)
	var env struct {
		Data struct {
			Chat dto.AgentChatDTO `json:"chat"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "c1", env.Data.Chat.ParentID)
	assert.Equal(t, 3, env.Data.Chat.Order)
}

func TestPlaceChat_ReturnsTheShiftedFolders(t *testing.T) {
	tree := &fakeChatTree{
		placed:  domain.Chat{ID: "c2", WorkspaceID: "ws-1"},
		shifted: []domain.Chat{{ID: "f0", Type: domain.ChatTypeFolder, Order: 1}},
	}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement", []byte(`{"order":0}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c2"}}

	newFolderHandlers(tree).PlaceChat(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Data struct {
			Shifted []dto.AgentChatDTO `json:"shifted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data.Shifted, 1)
	assert.Equal(t, "f0", env.Data.Shifted[0].ID)
}

func TestPlaceChat_NoPathWorkspace_ResolvesWorkspaceFromTheChatItself(t *testing.T) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "c2", WorkspaceID: "ws-9", ParentID: "c1"}}
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c2", WorkspaceID: "ws-9"}}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement", []byte(`{"parentId":"c1"}`))
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c2"}}

	newFolderHandlersWith(uc, tree).PlaceChat(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-9", tree.gotID, "PlaceChat must be called with the chat's OWN workspace")
	assert.Equal(t, "c2", tree.gotPlaceID)
}

func TestPlaceChat_NoPathWorkspace_GetChatErrorSurfaces(t *testing.T) {
	tree := &fakeChatTree{}
	uc := &fakeAgentUsecase{getChatErr: problem.ErrNotFound}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement", []byte(`{"parentId":"c1"}`))
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c2"}}

	newFolderHandlersWith(uc, tree).PlaceChat(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, tree.gotPlaceID, "the tree usecase must never be reached")
}

func TestPlaceChat_CycleIsAConflict(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeCycle}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c1/placement", []byte(`{"parentId":"c2"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c1"}}

	newFolderHandlers(tree).PlaceChat(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestPlaceChat_MalformedBodyIs400(t *testing.T) {
	tree := &fakeChatTree{}
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c1/placement", []byte(`{`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c1"}}

	newFolderHandlers(tree).PlaceChat(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, tree.gotPlaceID, "a malformed body must not reach the usecase")
}

func decodeFolderResponse(
	t *testing.T,
	raw []byte,
) struct {
	Folder  dto.AgentChatDTO   `json:"folder"`
	Shifted []dto.AgentChatDTO `json:"shifted"`
} {
	t.Helper()
	var env struct {
		Data struct {
			Folder  dto.AgentChatDTO   `json:"folder"`
			Shifted []dto.AgentChatDTO `json:"shifted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env))
	return env.Data
}

func homeParams(
	extra ...gin.Param,
) gin.Params {
	return append(gin.Params{{Key: "repoId", Value: ""}, {Key: "wsId", Value: "home-A"}}, extra...)
}

func TestRegression_ListFolders_OnTheHomeMountListsThatHomeOnly(t *testing.T) {
	tree := &fakeChatTree{list: []domain.Chat{{ID: "f-A", Type: domain.ChatTypeFolder}}}
	ctx, rec := newTestContext(t, http.MethodGet, "/home/chats/folders", nil)
	ctx.Params = homeParams()

	newFolderHandlers(tree).ListFolders(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "home-A", tree.gotHomeID, "the home mount lists by its injected home workspace")
}

func TestRegression_PatchFolder_RefusesAnotherProjectsHomeFolder(t *testing.T) {
	tree := &fakeChatTree{
		moved:  domain.Chat{ID: "f-B", Type: domain.ChatTypeFolder},
		folder: tree.Folder{ID: "f-B", RepoID: "", HomeID: "home-B"},
	}
	ctx, rec := newTestContext(t, http.MethodPatch, "/home/chats/folders/f-B", []byte(`{"name":"stolen"}`))
	ctx.Params = homeParams(gin.Param{Key: "folderId", Value: "f-B"})

	newFolderHandlers(tree).PatchFolder(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Zero(t, tree.renames)
	assert.Zero(t, tree.moves)
}

func TestRegression_DeleteFolder_RefusesAnotherProjectsHomeFolder(t *testing.T) {
	tree := &fakeChatTree{folder: tree.Folder{ID: "f-B", RepoID: "", HomeID: "home-B"}}
	ctx, rec := newTestContext(t, http.MethodDelete, "/home/chats/folders/f-B", nil)
	ctx.Params = homeParams(gin.Param{Key: "folderId", Value: "f-B"})

	newFolderHandlers(tree).DeleteFolder(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, tree.gotID, "nothing may be deleted")
}

func TestRegression_CreateFolder_OnTheHomeMountRecordsItsHome(t *testing.T) {
	tree := &fakeChatTree{created: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Title: "docs"}}
	ctx, rec := newTestContext(t, http.MethodPost, "/home/chats/folders", []byte(`{"name":"docs"}`))
	ctx.Params = homeParams()

	newFolderHandlers(tree).CreateFolder(ctx)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "", tree.gotCreate.RepoID)
	assert.Equal(t, "home-A", tree.gotCreate.HomeID)
}
