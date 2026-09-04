package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeChatTree records each call and returns canned results, so the handlers'
// HTTP contract can be pinned without a store.
type fakeChatTree struct {
	list         []domain.Chat
	created      domain.Chat
	renamed      domain.Chat
	moved        domain.Chat
	placed       domain.Chat
	shifted      []domain.Chat
	deletion     agentusecase.ChatDeletion
	previewChats int
	previewFiles int
	gotPreviewID string
	err          error

	gotCreate  agentusecase.CreateInput
	gotMove    agentusecase.MoveInput
	gotPlace   agentusecase.PlaceInput
	gotRepoID  string
	gotRename  string
	gotID      string
	gotPlaceID string
	gotCreate2 createChatCall
	gotPurge   string
	renames    int
	moves      int
	// The WHOLE worktree spec the create carried, and how many creates ran —
	// what an IMPORT is asserted through, since it flattens to the same
	// OwnWorktree=false a plain chat does.
	gotWorktree     agentusecase.WorktreeSpec
	createChatCalls int
}

func (f *fakeChatTree) ListInRepo(
	_ context.Context,
	repoID string,
) ([]domain.Chat, error) {
	f.gotRepoID = repoID
	return f.list, f.err
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

// createChatCall is the argument quadruple a chat create carries, recorded
// whole: which provider, WHERE the chat is born, and whether it asked for its
// own worktree. The parent is the half that decides whether the new chat is a
// thread, so a handler that dropped it would still look like a working create.
type createChatCall struct {
	WorkspaceID string
	ProviderID  string
	ParentID    string
	OwnWorktree bool
}

// CreateChat records the call twice over: gotCreate2 keeps the flattened
// "is this a fork" bool every pre-existing assertion here is written against,
// and gotWorktree keeps the WHOLE three-state spec, which is the only way to
// tell a WorktreeImport from a plain chat (both flatten to OwnWorktree=false).
func (f *fakeChatTree) CreateChat(
	_ context.Context,
	workspaceID string,
	providerID string,
	parentID string,
	worktree agentusecase.WorktreeSpec,
) (string, string, error) {
	f.gotCreate2 = createChatCall{
		WorkspaceID: workspaceID,
		ProviderID:  providerID,
		ParentID:    parentID,
		OwnWorktree: worktree.Mode == agentusecase.WorktreeFork,
	}
	f.gotWorktree = worktree
	f.createChatCalls++
	return f.placed.ID, "runner-1", f.err
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

func (f *fakeChatTree) DeleteChat(
	_ context.Context,
	chatID string,
) (agentusecase.ChatDeletion, error) {
	f.gotPurge = chatID
	return f.deletion, f.err
}

func (f *fakeChatTree) DeletePreview(
	_ context.Context,
	chatID string,
) (int, int, error) {
	f.gotPreviewID = chatID
	return f.previewChats, f.previewFiles, f.err
}

// folderFrame is one chat-folder frame the handlers pushed on the Chats socket.
type folderFrame struct {
	folderID    string
	workspaceID string
	kind        string
}

// agentUsecases is every agent port the handler set takes, behind one type, so
// one test double can still stand in for all five. Production wires them
// separately — that is the whole point of the split — and each double here is
// handed to New under each port in turn.
type agentUsecases interface {
	handlers.ChatUsecase
	handlers.TurnUsecase
	handlers.RunnerUsecase
	handlers.AnswerUsecase
	handlers.ProviderUsecase
}

// newChatHandlers builds the handler set the chat tests use: a real agent
// usecase double, an inert tree, and no broadcaster.
func newChatHandlers(
	uc agentUsecases,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, &fakeChatTree{}, nil)
}

// newChatHandlersWith is newChatHandlers with the tree double the caller holds,
// for the create tests: a chat create is now a TREE operation (it has to place the
// row before a CLI is started on it), so what the handler forwarded is recorded
// there and nowhere else.
func newChatHandlersWith(
	uc agentUsecases,
	tree *fakeChatTree,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, tree, nil)
}

// newFolderHandlers builds the handler set the tree tests use, capturing every
// frame the mutation fans out.
func newFolderHandlers(
	tree *fakeChatTree,
	frames *[]folderFrame,
) *handlers.Handlers {
	return newFolderHandlersWith(&fakeAgentUsecase{}, tree, frames)
}

// newFolderHandlersWith is newFolderHandlers with a caller-supplied agent
// usecase, for the routes that read the chat as well as the tree.
func newFolderHandlersWith(
	uc agentUsecases,
	tree *fakeChatTree,
	frames *[]folderFrame,
) *handlers.Handlers {
	return handlers.New(uc, uc, uc, uc, uc, tree, func(folderID, workspaceID, kind string) {
		*frames = append(*frames, folderFrame{folderID: folderID, workspaceID: workspaceID, kind: kind})
	})
}

// folderParams sets both :repoId (what the folder CRUD verbs scope by) and
// :wsId (what the broadcast callback still names), mirroring the real router's
// nesting where both are always present together.
func folderParams(
	extra ...gin.Param,
) gin.Params {
	return append(gin.Params{{Key: "repoId", Value: "r1"}, {Key: "wsId", Value: "ws-1"}}, extra...)
}

// The URL scope is authoritative: a POST against one repo must never create a
// folder in another, so the body carries no repo at all.
func TestCreateFolder_TakesTheScopeFromTheURL(t *testing.T) {
	tree := &fakeChatTree{created: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Title: "spikes"}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders",
		[]byte(`{"name":"spikes","parentId":"c1","repoId":"evil"}`))
	ctx.Params = folderParams()

	newFolderHandlers(tree, &frames).CreateFolder(ctx)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "r1", tree.gotCreate.RepoID)
	assert.Equal(t, "c1", tree.gotCreate.ParentID)
	require.Len(t, frames, 1)
	assert.Equal(t, folderFrame{folderID: "f1", workspaceID: "ws-1", kind: "folder_created"}, frames[0])
}

// A create densifies the level, so the rows it shifted have to reach the client
// too — otherwise their orders stay stale until the next reconnect.
func TestCreateFolder_ReturnsAndAnnouncesTheCollateral(t *testing.T) {
	tree := &fakeChatTree{
		created: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder},
		shifted: []domain.Chat{{ID: "f0", Type: domain.ChatTypeFolder, Order: 0}},
	}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{"name":"spikes"}`))
	ctx.Params = folderParams()

	newFolderHandlers(tree, &frames).CreateFolder(ctx)

	require.Equal(t, http.StatusCreated, rec.Code)
	body := decodeFolderResponse(t, rec.Body.Bytes())
	assert.Equal(t, "f1", body.Folder.ID)
	require.Len(t, body.Shifted, 1)
	assert.Equal(t, "f0", body.Shifted[0].ID)
	require.Len(t, frames, 2)
	assert.Equal(t, "folder_updated", frames[0].kind, "the collateral is announced before the subject")
	assert.Equal(t, "folder_created", frames[1].kind)
}

func TestCreateFolder_SurfacesTheUsecaseRefusal(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeNameRequired}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{"name":" "}`))
	ctx.Params = folderParams()

	newFolderHandlers(tree, &frames).CreateFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, frames, "a refused create announces nothing")
}

func TestCreateFolder_MalformedBodyIs400(t *testing.T) {
	tree := &fakeChatTree{}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{`))
	ctx.Params = folderParams()

	newFolderHandlers(tree, &frames).CreateFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, tree.gotCreate.RepoID, "a malformed body must not reach the usecase")
}

func TestListFolders_ScopesToTheURLRepo(t *testing.T) {
	tree := &fakeChatTree{list: []domain.Chat{
		{ID: "b", Type: domain.ChatTypeFolder, Order: 1},
		{ID: "a", Type: domain.ChatTypeFolder, Order: 0},
	}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodGet, "/chats/folders", nil)
	ctx.Params = folderParams()

	newFolderHandlers(tree, &frames).ListFolders(ctx)

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
	tree := &fakeChatTree{err: apperr.ErrNotFound}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodGet, "/chats/folders", nil)
	ctx.Params = folderParams()

	newFolderHandlers(tree, &frames).ListFolders(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// A drag that renames AND moves must land as one answer, not two half-states.
func TestPatchFolder_RenameThenMove(t *testing.T) {
	tree := &fakeChatTree{moved: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Title: "new", Order: 2}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1",
		[]byte(`{"name":"new","parentId":"c1","order":2}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, tree.renames)
	assert.Equal(t, 1, tree.moves)
	assert.Equal(t, "new", tree.gotRename)
	require.NotNil(t, tree.gotMove.ParentID)
	assert.Equal(t, "c1", *tree.gotMove.ParentID)
	require.NotNil(t, tree.gotMove.Order)
	assert.Equal(t, 2, *tree.gotMove.Order)
	require.Len(t, frames, 1, "one gesture, one frame")
	assert.Equal(t, "folder_updated", frames[0].kind)
}

// A PATCH that reorders within one parent carries no parentId, and the nil must
// reach the usecase as "leave it where it is" rather than "move it to the root".
func TestPatchFolder_OrderOnlyLeavesTheParentNil(t *testing.T) {
	tree := &fakeChatTree{moved: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder, Order: 1}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"order":1}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Zero(t, tree.renames)
	assert.Nil(t, tree.gotMove.ParentID)
	require.NotNil(t, tree.gotMove.Order)
	assert.Equal(t, 1, *tree.gotMove.Order)
}

// An explicit empty parentId is a MOVE TO THE PANEL ROOT, and must be
// distinguishable from an absent one.
func TestPatchFolder_EmptyParentMeansThePanelRoot(t *testing.T) {
	tree := &fakeChatTree{moved: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"parentId":""}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).PatchFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, tree.gotMove.ParentID)
	assert.Equal(t, "", *tree.gotMove.ParentID)
}

// A failed rename must stop before the placement runs, or a refused PATCH would
// still half-apply.
func TestPatchFolder_FailedRenameSkipsTheMove(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeNameRequired}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"name":" ","order":0}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).PatchFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, 1, tree.renames)
	assert.Zero(t, tree.moves, "the placement must not run after a refused rename")
	assert.Empty(t, frames)
}

func TestPatchFolder_CycleIsAConflict(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeCycle}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{"parentId":"f2"}`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).PatchFolder(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, frames)
}

func TestPatchFolder_MalformedBodyIs400(t *testing.T) {
	tree := &fakeChatTree{}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/folders/f1", []byte(`{`))
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).PatchFolder(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, tree.moves)
}

// The promoted rows are what stop a folder's children vanishing with it; the
// tombstone frame is what makes the client drop the folder itself.
func TestDeleteFolder_AnnouncesThePromotedRowsThenTheTombstone(t *testing.T) {
	tree := &fakeChatTree{shifted: []domain.Chat{{ID: "child", Type: domain.ChatTypeFolder}}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodDelete, "/chats/folders/f1", nil)
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).DeleteFolder(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "f1", tree.gotID)
	require.Len(t, frames, 2)
	assert.Equal(t, folderFrame{folderID: "child", workspaceID: "ws-1", kind: "folder_updated"}, frames[0])
	assert.Equal(t, folderFrame{folderID: "f1", workspaceID: "ws-1", kind: "folder_deleted"}, frames[1])
}

func TestDeleteFolder_NotFoundIs404(t *testing.T) {
	tree := &fakeChatTree{err: apperr.ErrNotFound}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodDelete, "/chats/folders/f1", nil)
	ctx.Params = folderParams(gin.Param{Key: "folderId", Value: "f1"})

	newFolderHandlers(tree, &frames).DeleteFolder(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, frames, "a failed delete must not tombstone a folder that still exists")
}

// The placement endpoint is where a chat becomes a THREAD of another, so the
// parent it is given has to reach the usecase verbatim.
func TestPlaceChat_ForwardsTheRequestedLineage(t *testing.T) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "c2", WorkspaceID: "ws-1", ParentID: "c1", Order: 3}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement",
		[]byte(`{"parentId":"c1","order":3}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c2"}}

	newFolderHandlers(tree, &frames).PlaceChat(ctx)

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

// A chat drop renumbers a level chats and folders SHARE, so the folder rows it
// moved ride back with the answer and are announced.
func TestPlaceChat_ReturnsAndAnnouncesTheShiftedFolders(t *testing.T) {
	tree := &fakeChatTree{
		placed:  domain.Chat{ID: "c2", WorkspaceID: "ws-1"},
		shifted: []domain.Chat{{ID: "f0", Type: domain.ChatTypeFolder, Order: 1}},
	}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement", []byte(`{"order":0}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c2"}}

	newFolderHandlers(tree, &frames).PlaceChat(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Data struct {
			Shifted []dto.AgentChatDTO `json:"shifted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data.Shifted, 1)
	assert.Equal(t, "f0", env.Data.Shifted[0].ID)
	require.Len(t, frames, 1)
	assert.Equal(t, "folder_updated", frames[0].kind)
}

// TestPlaceChat_NoPathWorkspace_ResolvesWorkspaceFromTheChatItself proves that
// at the repo-scoped mount (Task 17: no :wsId path param) PlaceChat resolves
// the chat's own current workspace via GetChat rather than trusting the URL,
// so the tree usecase's move-across-workspace assertion is asked about the
// chat's real workspace and not a stale/absent one.
func TestPlaceChat_NoPathWorkspace_ResolvesWorkspaceFromTheChatItself(t *testing.T) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "c2", WorkspaceID: "ws-9", ParentID: "c1"}}
	uc := &fakeAgentUsecase{getChat: domain.Chat{ID: "c2", WorkspaceID: "ws-9"}}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement", []byte(`{"parentId":"c1"}`))
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c2"}}

	newFolderHandlersWith(uc, tree, &frames).PlaceChat(ctx)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ws-9", tree.gotID, "PlaceChat must be called with the chat's OWN workspace")
	assert.Equal(t, "c2", tree.gotPlaceID)
}

// TestPlaceChat_NoPathWorkspace_GetChatErrorSurfaces proves a GetChat failure
// during that fallback resolution surfaces as a mapped error rather than
// reaching the tree usecase with a guessed workspace.
func TestPlaceChat_NoPathWorkspace_GetChatErrorSurfaces(t *testing.T) {
	tree := &fakeChatTree{}
	uc := &fakeAgentUsecase{getChatErr: apperr.ErrNotFound}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c2/placement", []byte(`{"parentId":"c1"}`))
	ctx.Params = gin.Params{{Key: "repoId", Value: "r1"}, {Key: "id", Value: "c2"}}

	newFolderHandlersWith(uc, tree, &frames).PlaceChat(ctx)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, tree.gotPlaceID, "the tree usecase must never be reached")
}

func TestPlaceChat_CycleIsAConflict(t *testing.T) {
	tree := &fakeChatTree{err: agentusecase.ErrTreeCycle}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c1/placement", []byte(`{"parentId":"c2"}`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c1"}}

	newFolderHandlers(tree, &frames).PlaceChat(ctx)

	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, frames)
}

func TestPlaceChat_MalformedBodyIs400(t *testing.T) {
	tree := &fakeChatTree{}
	var frames []folderFrame
	ctx, rec := newTestContext(t, http.MethodPatch, "/chats/c1/placement", []byte(`{`))
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}, {Key: "id", Value: "c1"}}

	newFolderHandlers(tree, &frames).PlaceChat(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, tree.gotPlaceID, "a malformed body must not reach the usecase")
}

// A nil broadcast must degrade to a no-op rather than panic, matching every
// other handler wired without a hub.
func TestNew_NilFolderBroadcastDegradesToNoop(t *testing.T) {
	uc := &fakeAgentUsecase{}
	h := handlers.New(uc, uc, uc, uc, uc,
		&fakeChatTree{created: domain.Chat{ID: "f1", Type: domain.ChatTypeFolder}}, nil)
	ctx, rec := newTestContext(t, http.MethodPost, "/chats/folders", []byte(`{"name":"spikes"}`))
	ctx.Params = folderParams()

	assert.NotPanics(t, func() { h.CreateFolder(ctx) })
	assert.Equal(t, http.StatusCreated, rec.Code)
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
