package chat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeChatUsecase is a minimal agentusecase.ChatUsecase stand-in exposing
// only what NewHomeCorrectedChats reads (ListChats/ListChatsByWorkspace/
// ListChatsInRepo/GetChat) — every other method panics if ever called, which
// is itself the proof the decorator delegates everything else untouched
// (none of these tests exercise them).
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

func (f *fakeChatUsecase) MintChat(context.Context, string) (string, error) { panic("unused") }
func (f *fakeChatUsecase) RenameChat(context.Context, string, string, string) error {
	panic("unused")
}

func (f *fakeChatUsecase) RenameByRunner(context.Context, string, string, string) error {
	panic("unused")
}
func (f *fakeChatUsecase) PurgeChat(context.Context, string) error { panic("unused") }
func (f *fakeChatUsecase) CwdWorkspaceID(context.Context, string) (string, bool, error) {
	panic("unused")
}

func (f *fakeChatUsecase) SetChatSelection(context.Context, string, string, string) error {
	panic("unused")
}

func (f *fakeChatUsecase) ReadChatLog(context.Context, string) ([]agenttools.ChatTurn, error) {
	panic("unused")
}

func (f *fakeChatUsecase) ReadMessages(context.Context, string, int, int, int) (domain.LedgerPage, error) {
	panic("unused")
}

func (f *fakeChatUsecase) NoteThreadLineage(context.Context, string, []string) error {
	panic("unused")
}
func (f *fakeChatUsecase) Ancestors(context.Context, string) ([]string, error) { panic("unused") }
func (f *fakeChatUsecase) AssembleHandoff(context.Context, string) (string, error) {
	panic("unused")
}

func (f *fakeChatUsecase) Promote(context.Context, string) (domain.Chat, error) {
	panic("unused")
}

const homeWorkspaceID = "home-ws-1"

// TestNewHomeCorrectedChats_OverlaysLiveNodePositionForAHomeScopedChat is
// Critical 1's regression test: RED before the fix (ListChatsByWorkspace
// answered the CHAT aggregate's own frozen ParentID/Order, "" / 0, forever —
// exactly the "chat renders at the sidebar root permanently, not fixed by a
// reload" bug the SDD review caught), GREEN after (the wrapped usecase
// overlays the row's live Node position for a home-scoped chat).
func TestNewHomeCorrectedChats_OverlaysLiveNodePositionForAHomeScopedChat(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		// Frozen at creation, per this task's own write-once contract: the
		// row's REAL container (a home folder) never got written back here.
		{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "c1", Kind: domain.NodeKindChat, ParentID: "folder-1", Order: 2},
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

// A repo-scoped chat's placement is Node-backed too now (2026-09-08
// sidebar-placement-unification Task 8 widens Critical 1's fix from
// home-only to every non-bubble workspace) — the decorator must overlay its
// live Node position exactly as it already does for a home-scoped chat, not
// leave it at its stale Chat.ParentID/.Order.
func TestNewHomeCorrectedChats_OverlaysLiveNodePositionForARepoScopedChatToo(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c2", Type: domain.ChatTypeChat, WorkspaceID: "ws-repo-1", ParentID: "some-branch", Order: 5},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "c2", Kind: domain.NodeKindChat, ParentID: "a-repo-scoped-folder", Order: 1},
	}

	corrected := agentusecase.NewHomeCorrectedChats(inner, nodes)

	got, err := corrected.GetChat(context.Background(), "c2")
	require.NoError(t, err)
	assert.Equal(t, "a-repo-scoped-folder", got.ParentID, "the live Node position, not the frozen Chat field")
	assert.Equal(t, 1, got.Order)
}

// A bubble (WorkspaceID == "") is never home-scoped -- correcting it would
// mean calling RepoOf("") and GetNode for a row that (per this task's own
// scope boundary) never gets a Node mint at all.
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

// A home-scoped chat with no Node row yet (its very first placement is still
// in flight, or it was minted via the bare SpawnChat path this task's report
// already discloses as unminting) degrades to its Chat-native placement
// rather than erroring the whole read.
func TestNewHomeCorrectedChats_DegradesWhenNoNodeRowExistsYet(t *testing.T) {
	inner := &fakeChatUsecase{rows: []domain.Chat{
		{ID: "c4", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements() // no rows -- GetNode answers not-found

	corrected := agentusecase.NewHomeCorrectedChats(inner, nodes)

	got, err := corrected.GetChat(context.Background(), "c4")
	require.NoError(t, err)
	assert.Equal(t, "", got.ParentID)
	assert.Equal(t, 0, got.Order)
}

// fakeTreeChats is a minimal tree.Chats stand-in exposing only the two reads
// NewHomeCorrectedTreeChats overrides (LoadChat, ListByWorkspace) — every
// other method panics if ever called, proving the decorator delegates the
// rest (Create/SetTitle/SetPlacement/SetOrder/SetType/Forget/Get/ListChats)
// untouched.
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
func (f *fakeTreeChats) Create(context.Context, agentchat.CreateInput) (domain.Chat, error) {
	panic("unused")
}

func (f *fakeTreeChats) SetTitle(context.Context, string, string, string) (domain.Chat, error) {
	panic("unused")
}

func (f *fakeTreeChats) SetPlacement(context.Context, string, string, int) (domain.Chat, error) {
	panic("unused")
}

func (f *fakeTreeChats) SetOrder(context.Context, string, int) (domain.Chat, error) {
	panic("unused")
}

func (f *fakeTreeChats) SetType(context.Context, string, domain.ChatType) (domain.Chat, error) {
	panic("unused")
}
func (f *fakeTreeChats) Forget(context.Context, string) error { panic("unused") }

// TestNewHomeCorrectedTreeChats_LoadChatOverlaysLiveNodePosition is the
// lineage-resolver half of this fix round: internal/lineage.Resolver.
// Ancestors (what a freshly spawned CLI is told to read) calls LoadChat
// FIRST and early-exits on chat.ParentID == "" -- RED before this fix, since
// LoadChat answered the frozen Chat field forever for a home-scoped chat
// moved under a folder; GREEN after, the live Node position is overlaid.
func TestNewHomeCorrectedTreeChats_LoadChatOverlaysLiveNodePosition(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "c1", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "c1", Kind: domain.NodeKindChat, ParentID: "folder-1", Order: 3},
	}

	corrected := agentusecase.NewHomeCorrectedTreeChats(inner, nodes, mocks.NewFolderStore())

	got, err := corrected.LoadChat(context.Background(), "c1")
	require.NoError(t, err)
	assert.Equal(t, "folder-1", got.ParentID, "the live Node position, not the frozen Chat field")
	assert.Equal(t, 3, got.Order)
}

// TestNewHomeCorrectedTreeChats_FeedsCorrectAncestryToTheRealLineageResolver
// runs internal/lineage's ACTUAL Resolver (via tree.NewLineage/ChatLineage,
// the same construction container.go uses) over the wrapped Chats port, to
// prove the fix closes the real bug end to end, not merely the two wrapper
// methods in isolation: a home-scoped thread moved under a chat now reports
// that chat as its ancestor, where before the fix it silently reported none
// at all (LoadChat's own early exit on a frozen, empty ParentID).
func TestNewHomeCorrectedTreeChats_FeedsCorrectAncestryToTheRealLineageResolver(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "parent-chat", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
		// thread's OWN Chat.ParentID is frozen at "" (its creation-time bubble
		// value) even though it was later dragged under parent-chat -- the
		// write-once-then-ignored contract this task's report discloses.
		{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "parent-chat", Kind: domain.NodeKindChat, ParentID: "", Order: 0},
		{ID: "thread", Kind: domain.NodeKindChat, ParentID: "parent-chat", Order: 0},
	}

	corrected := agentusecase.NewHomeCorrectedTreeChats(inner, nodes, mocks.NewFolderStore())
	lineage := agentusecase.NewChatLineage(corrected)

	ancestors, err := lineage.Ancestors(context.Background(), "thread")
	require.NoError(t, err)
	assert.Equal(t, []string{"parent-chat"}, ancestors,
		"a freshly spawned CLI on this thread must be told it reads parent-chat's turns")
}

// TestNewHomeCorrectedTreeChats_FeedsAncestryThroughAFolder is Task 8's own
// addition: a folder is Folder/Node-backed now, never a Chat row, so it no
// longer arrives through ListByWorkspace's raw read at all — RED before
// foldersReachableFrom (internal/lineage.Resolver's own walk finds no
// ParentID entry for the folder id, so it stops there and reports no
// ancestor), GREEN after (the folder is folded into the list so the walk
// steps straight through it, exactly as it already does for an ordinary
// chat-typed container).
func TestNewHomeCorrectedTreeChats_FeedsAncestryThroughAFolder(t *testing.T) {
	inner := &fakeTreeChats{rows: []domain.Chat{
		{ID: "parent-chat", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
		{ID: "thread", Type: domain.ChatTypeChat, WorkspaceID: homeWorkspaceID, ParentID: "", Order: 0},
	}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []domain.Node{
		{ID: "parent-chat", Kind: domain.NodeKindChat, ParentID: "", Order: 0},
		{ID: "a-folder", Kind: domain.NodeKindFolder, ParentID: "parent-chat", Order: 0},
		{ID: "thread", Kind: domain.NodeKindChat, ParentID: "a-folder", Order: 0},
	}
	folders := mocks.NewFolderStore()
	folders.Saved = []domain.Folder{{ID: "a-folder", Name: "notes"}}

	corrected := agentusecase.NewHomeCorrectedTreeChats(inner, nodes, folders)
	lineage := agentusecase.NewChatLineage(corrected)

	ancestors, err := lineage.Ancestors(context.Background(), "thread")
	require.NoError(t, err)
	assert.Equal(t, []string{"parent-chat"}, ancestors,
		"the folder between thread and parent-chat must be stepped through, not lost")
}

var _ tree.Chats = (*fakeTreeChats)(nil)

// TestRegression_UncorrectedTreeChatsFeedsNoAncestryToTheRealLineageResolver
// pins the RED half directly, kept as a permanent regression: the SAME
// scenario as TestNewHomeCorrectedTreeChats_FeedsCorrectAncestryToTheRealLineageResolver
// above, but against the RAW (uncorrected) tree.Chats a caller might wire by
// mistake -- reproducing the actual pre-fix bug (a freshly spawned CLI told
// it has no prior context, when it does) rather than merely asserting a lack
// of test coverage.
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
