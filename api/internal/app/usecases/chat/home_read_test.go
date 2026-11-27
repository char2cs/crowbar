package chat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	dtree "github.com/char2cs/crowbar/api/internal/domain/tree"
)

type fakeChatUsecase struct {
	rows []domain.Chat
}

func (f *fakeChatUsecase) ListChats(context.Context) ([]domain.Chat, error) {
	return append([]domain.Chat{}, f.rows...), nil
}

func (f *fakeChatUsecase) ListChatsByWorkspace(_ context.Context, workspaceID string) ([]domain.Chat, error) {
	out := make([]domain.Chat, 0, len(f.rows))
	for _, r := range f.rows {
		if r.WorkspaceID == workspaceID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeChatUsecase) BackfillChatTypes(context.Context) (int, error) { panic("unused") }

func (f *fakeChatUsecase) UntypedChats(context.Context) ([]string, error) { panic("unused") }

func (f *fakeChatUsecase) ListChatsInRepo(context.Context, string) ([]domain.Chat, error) {
	return append([]domain.Chat{}, f.rows...), nil
}

func (f *fakeChatUsecase) GetChat(_ context.Context, id string) (domain.Chat, error) {
	for _, r := range f.rows {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.Chat{}, errors.New("not found")
}

func (f *fakeChatUsecase) MintChat(context.Context, string, string, string, string) (string, error) {
	panic("unused")
}

func (f *fakeChatUsecase) ChatSnapshot(context.Context, string) (agentusecase.ChatSnapshot, error) {
	panic("unused")
}

func (f *fakeChatUsecase) ChatSnapshotsOf(context.Context, []string) ([]agentusecase.ChatSnapshot, error) {
	panic("unused")
}

func (f *fakeChatUsecase) RenameChat(context.Context, string, string, string) error {
	panic("unused")
}

func (f *fakeChatUsecase) RenameByRunner(context.Context, string, string, string) error {
	panic("unused")
}
func (f *fakeChatUsecase) PurgeChat(context.Context, string) error { panic("unused") }

func (f *fakeChatUsecase) OpenFeed(context.Context, string, int64, int64) (<-chan agentusecase.FeedFrame, error) {
	panic("unused")
}

func (f *fakeChatUsecase) ReadRows(context.Context, string, int64, int64) (agentusecase.RowsPage, error) {
	panic("unused")
}

func (f *fakeChatUsecase) ReadRow(context.Context, string, int64) (ledger.Row, error) {
	panic("unused")
}

func (f *fakeChatUsecase) ReadBlob(context.Context, string, string) ([]byte, error) {
	panic("unused")
}

func (f *fakeChatUsecase) CwdWorkspaceID(context.Context, string) (string, bool, error) {
	panic("unused")
}

func (f *fakeChatUsecase) SetChatSelection(context.Context, string, string, string) error {
	panic("unused")
}

func (f *fakeChatUsecase) ReadChatLog(context.Context, string) ([]agenttools.ChatTurn, error) {
	panic("unused")
}

func (f *fakeChatUsecase) NoteThreadLineage(context.Context, string, []string) error {
	panic("unused")
}
func (f *fakeChatUsecase) Ancestors(context.Context, string) ([]string, error) { panic("unused") }
func (f *fakeChatUsecase) AssembleHandoff(context.Context, string) (string, error) {
	panic("unused")
}

const homeWorkspaceID = "home-ws-1"

func TestNewHomeCorrectedChats_OverlaysLiveNodePositionForAHomeScopedChat(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "c1", Kind: dtree.KindChat, ParentID: "folder-1", Order: 2},
	}

	corrected := agentusecase.NewHomeCorrectedChats(inner, nodes)

	rows, err := corrected.ListChatsByWorkspace(context.Background(), homeWorkspaceID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "folder-1", rows[0].ParentID, "the live Node position, not the frozen Chat field")
	assert.Equal(t, 2, rows[0].Order)

	got, err := corrected.GetChat(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, "folder-1", got.ParentID, "GetChat is corrected too, not just the list path")
	assert.Equal(t, 2, got.Order)

	all, err := corrected.ListChats(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "folder-1", all[0].ParentID, "ListChats is corrected too")

	inRepo, err := corrected.ListChatsInRepo(context.Background(), "some-repo")
	require.NoError(t, err)
	require.Len(t, inRepo, 1)
	assert.Equal(t, "folder-1", inRepo[0].ParentID, "ListChatsInRepo is corrected too")
}

func TestNewHomeCorrectedChats_OverlaysLiveNodePositionForARepoScopedChatToo(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: "ws-repo-1", ParentID: "some-branch", Order: 5},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "c2", Kind: dtree.KindChat, ParentID: "a-repo-scoped-folder", Order: 1},
	}

	corrected := agentusecase.NewHomeCorrectedChats(inner, nodes)

	got, err := corrected.GetChat(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, "a-repo-scoped-folder", got.ParentID, "the live Node position, not the frozen Chat field")
	assert.Equal(t, 1, got.Order)
}

func TestNewHomeCorrectedChats_LeavesABubbleUntouched(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c3", Type: domain.ChatTypeChat, WorkspaceID: "", ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()

	corrected := agentusecase.NewHomeCorrectedChats(inner, nodes)

	got, err := corrected.GetChat(context.Background(), "c3")
	require.NoError(t, err)
	assert.Equal(t, "", got.ParentID)
	assert.Equal(t, 0, got.Order)
}

func TestNewHomeCorrectedChats_DegradesWhenNoNodeRowExistsYet(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c4", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()

	corrected := agentusecase.NewHomeCorrectedChats(inner, nodes)

	got, err := corrected.GetChat(context.Background(), "c4")
	require.NoError(t, err)
	assert.Equal(t, "", got.ParentID)
	assert.Equal(t, 0, got.Order)
}

type fakeTreeChats struct {
	rows []domain.Chat
}

func (f *fakeTreeChats) LoadChat(_ context.Context, id string) (domain.Chat, error) {
	for _, r := range f.rows {
		if r.ID == id {
			return r, nil
		}
	}
	return domain.Chat{}, errors.New("not found")
}

func (f *fakeTreeChats) ListByWorkspace(_ context.Context, workspaceID string) ([]domain.Chat, error) {
	out := make([]domain.Chat, 0, len(f.rows))
	for _, r := range f.rows {
		if r.WorkspaceID == workspaceID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeTreeChats) ListChats(context.Context) ([]domain.Chat, error) { panic("unused") }
func (f *fakeTreeChats) Get(context.Context, string) (domain.Chat, error) { panic("unused") }
func (f *fakeTreeChats) SetPlacement(context.Context, string, string, int) (domain.Chat, error) {
	panic("unused")
}

func (f *fakeTreeChats) SetOrder(context.Context, string, int) (domain.Chat, error) {
	panic("unused")
}

func TestNewHomeCorrectedTreeChats_LoadChatOverlaysLiveNodePosition(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "c1", Kind: dtree.KindChat, ParentID: "folder-1", Order: 3},
	}

	corrected := agentusecase.NewHomeCorrectedTreeChats(inner, nodes, mocks.NewFolderStore())

	got, err := corrected.LoadChat(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, "folder-1", got.ParentID, "the live Node position, not the frozen Chat field")
	assert.Equal(t, 3, got.Order)
}

func TestNewHomeCorrectedTreeChats_FeedsCorrectAncestryToTheRealLineageResolver(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "parent-chat", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
		{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "parent-chat", Kind: dtree.KindChat, ParentID: "", Order: 0},
		{ID: "thread", Kind: dtree.KindChat, ParentID: "parent-chat", Order: 0},
	}

	corrected := agentusecase.NewHomeCorrectedTreeChats(inner, nodes, mocks.NewFolderStore())
	lineage := agentusecase.NewChatLineage(corrected)

	ancestors, err := lineage.Ancestors(context.Background(), "thread")
	require.NoError(t, err)
	assert.Equal(t, []string{"parent-chat"}, ancestors,
		"a freshly spawned CLI on this thread must be told it reads parent-chat's turns")
}

func TestNewHomeCorrectedTreeChats_FeedsAncestryThroughAFolder(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "parent-chat", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
		{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []dtree.Node{
		{ID: "parent-chat", Kind: dtree.KindChat, ParentID: "", Order: 0},
		{ID: "a-folder", Kind: dtree.KindFolder, ParentID: "parent-chat", Order: 0},
		{ID: "thread", Kind: dtree.KindChat, ParentID: "a-folder", Order: 0},
	}
	folders := mocks.NewFolderStore()
	folders.Saved = []dtree.Folder{{ID: "a-folder", Name: "notes"}}

	corrected := agentusecase.NewHomeCorrectedTreeChats(inner, nodes, folders)
	lineage := agentusecase.NewChatLineage(corrected)

	ancestors, err := lineage.Ancestors(context.Background(), "thread")
	require.NoError(t, err)
	assert.Equal(t, []string{"parent-chat"}, ancestors,
		"the folder between thread and parent-chat must be stepped through, not lost")
}

var _ tree.Chats = (*fakeTreeChats)(nil)

func TestRegression_UncorrectedTreeChatsFeedsNoAncestryToTheRealLineageResolver(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "parent-chat", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
		{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	lineage := agentusecase.NewChatLineage(inner)

	ancestors, err := lineage.Ancestors(context.Background(), "thread")
	require.NoError(t, err)
	assert.Empty(t, ancestors,
		"reproduces the bug: the raw (uncorrected) frozen Chat.ParentID reports no ancestor at all")
}

type countingNodeReads struct {
	*mocks.NodePlacements
	gets, all int
}

func (c *countingNodeReads) GetNode(ctx context.Context, id string) (dtree.Node, error) {
	c.gets++
	return c.NodePlacements.GetNode(ctx, id)
}

func (c *countingNodeReads) ListAll(ctx context.Context) ([]dtree.Node, error) {
	c.all++
	return c.NodePlacements.ListAll(ctx)
}

func TestNewHomeCorrectedChats_CorrectsAListFromOneNodeRead(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: "ws"},
		{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: "ws"},
		{ID: "bubble", Type: domain.ChatTypeChat},
		{ID: "unplaced", Type: domain.ChatTypeChat, WorkspaceID: "ws", ParentID: "kept", Order: 4},
	}}
	nodes := &countingNodeReads{NodePlacements: mocks.NewNodePlacements()}
	nodes.Rows = []dtree.Node{
		{ID: "c1", Kind: dtree.KindChat, ParentID: "folder-1", Order: 2},
		{ID: "c2", Kind: dtree.KindChat, ParentID: "folder-2", Order: 3},
	}

	rows, err := agentusecase.NewHomeCorrectedChats(inner, nodes).ListChatsInRepo(context.Background(), "r")

	require.NoError(t, err)
	require.Len(t, rows, 4)
	assert.Equal(t, "folder-1", rows[0].ParentID)
	assert.Equal(t, 3, rows[1].Order)
	assert.Empty(t, rows[2].ParentID, "a bubble is left alone")
	assert.Equal(t, "kept", rows[3].ParentID, "a row with no Node keeps its own placement")
	assert.Equal(t, 0, nodes.gets)
	assert.Equal(t, 1, nodes.all)
}
