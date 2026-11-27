package worktree_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

func TestResolve_AChatThatOwnsItsOwnWorktreeResolvesToItselfWithoutWalkingAncestors(t *testing.T) {
	chats := &fakeChatAncestryReader{
		ancestry: map[string][]domain.Chat{
			"chat-1": {{ID: "chat-1", WorkspaceID: "ws-1"}},
		},
	}
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{
			"ws-1": {ID: "ws-1", Branch: "feature/own"},
		},
	}

	ws, err := worktree.Resolve(context.Background(), "chat-1", chats, workspaces)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if ws.ID != "ws-1" {
		t.Fatalf("resolved workspace = %q, want ws-1", ws.ID)
	}
	if got := workspaces.gets; len(got) != 1 || got[0] != "ws-1" {
		t.Fatalf("workspaces.Get calls = %v, want exactly one call for ws-1", got)
	}
}

func TestResolve_AnImmediateParentOwnsTheWorktree(t *testing.T) {
	chats := &fakeChatAncestryReader{
		ancestry: map[string][]domain.Chat{
			"chat-child": {
				{ID: "chat-child", WorkspaceID: ""},
				{ID: "chat-parent", WorkspaceID: "ws-parent"},
			},
		},
	}
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{
			"ws-parent": {ID: "ws-parent", Branch: "feature/parent"},
		},
	}

	ws, err := worktree.Resolve(context.Background(), "chat-child", chats, workspaces)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if ws.ID != "ws-parent" {
		t.Fatalf("resolved workspace = %q, want ws-parent", ws.ID)
	}
}

func TestResolve_AWorktreeSeveralLevelsUpIsFound(t *testing.T) {
	chats := &fakeChatAncestryReader{
		ancestry: map[string][]domain.Chat{
			"chat-leaf": {
				{ID: "chat-leaf", WorkspaceID: ""},
				{ID: "chat-mid", WorkspaceID: ""},
				{ID: "chat-root", WorkspaceID: "ws-root"},
			},
		},
	}
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{
			"ws-root": {ID: "ws-root", Branch: "main"},
		},
	}

	ws, err := worktree.Resolve(context.Background(), "chat-leaf", chats, workspaces)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if ws.ID != "ws-root" {
		t.Fatalf("resolved workspace = %q, want ws-root", ws.ID)
	}
}

func TestResolve_NoWorktreeAnywhereInAncestryReturnsTypedErrorNotAZeroValue(t *testing.T) {
	chats := &fakeChatAncestryReader{
		ancestry: map[string][]domain.Chat{
			"chat-bubble": {
				{ID: "chat-bubble", WorkspaceID: ""},
				{ID: "chat-parent", WorkspaceID: ""},
			},
		},
	}
	workspaces := &fakeWorkspaceReader{byID: map[string]domain.Workspace{}}

	ws, err := worktree.Resolve(context.Background(), "chat-bubble", chats, workspaces)
	if !errors.Is(err, worktree.ErrNoWorktreeInAncestry) {
		t.Fatalf("err = %v, want ErrNoWorktreeInAncestry", err)
	}
	if ws != (domain.Workspace{}) {
		t.Fatalf("workspace = %+v, want zero value alongside the error", ws)
	}
}

func TestResolve_EmptyAncestryReturnsTypedErrorNotAZeroValue(t *testing.T) {
	chats := &fakeChatAncestryReader{
		ancestry: map[string][]domain.Chat{
			"chat-orphan": {},
		},
	}
	workspaces := &fakeWorkspaceReader{byID: map[string]domain.Workspace{}}

	ws, err := worktree.Resolve(context.Background(), "chat-orphan", chats, workspaces)
	if !errors.Is(err, worktree.ErrNoWorktreeInAncestry) {
		t.Fatalf("err = %v, want ErrNoWorktreeInAncestry", err)
	}
	if ws != (domain.Workspace{}) {
		t.Fatalf("workspace = %+v, want zero value alongside the error", ws)
	}
}

func TestResolve_ChatAncestryReaderErrorIsSurfacedWithContextNotSwallowed(t *testing.T) {
	cause := errors.New("ancestry store unavailable")
	chats := &fakeChatAncestryReader{
		err: cause,
	}
	workspaces := &fakeWorkspaceReader{byID: map[string]domain.Workspace{}}

	ws, err := worktree.Resolve(context.Background(), "chat-1", chats, workspaces)
	if err == nil {
		t.Fatalf("Resolve returned nil error, want the ancestry failure wrapped")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want it to wrap %v", err, cause)
	}
	if ws != (domain.Workspace{}) {
		t.Fatalf("workspace = %+v, want zero value alongside the error", ws)
	}
}

func TestResolve_WorkspaceReaderErrorIsSurfacedWithContextNotSwallowed(t *testing.T) {
	cause := errors.New("workspace store unavailable")
	chats := &fakeChatAncestryReader{
		ancestry: map[string][]domain.Chat{
			"chat-1": {{ID: "chat-1", WorkspaceID: "ws-1"}},
		},
	}
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{},
		err:  cause,
	}

	ws, err := worktree.Resolve(context.Background(), "chat-1", chats, workspaces)
	if err == nil {
		t.Fatalf("Resolve returned nil error, want the workspace failure wrapped")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want it to wrap %v", err, cause)
	}
	if ws != (domain.Workspace{}) {
		t.Fatalf("workspace = %+v, want zero value alongside the error", ws)
	}
}

func TestResolve_AWorktreeOwningAncestorAcrossAFolderIsFound(t *testing.T) {
	lister := &fakeChatLister{
		rows: []domain.Chat{
			{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-f"},
			{ID: "chat-a", Type: domain.ChatTypeChat, ParentID: "", WorkspaceID: "ws-a"},
			{ID: "folder-f", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
		},
	}
	chats := worktree.NewChatTreeAncestryReader(lister, nil, nil)
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{
			"ws-a": {ID: "ws-a", Branch: "feature/a"},
		},
	}

	ws, err := worktree.Resolve(context.Background(), "chat-b", chats, workspaces)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if ws.ID != "ws-a" {
		t.Fatalf("resolved workspace = %q, want ws-a (chat-b's nearest worktree-owning ancestor across folder-f)", ws.ID)
	}
}

func TestResolve_AWorktreeOwningAncestorAcrossNestedFoldersIsFound(t *testing.T) {
	lister := &fakeChatLister{
		rows: []domain.Chat{
			{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
			{ID: "folder-outer", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
			{ID: "folder-inner", Type: domain.ChatTypeFolder, ParentID: "folder-outer"},
			{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-inner"},
		},
	}
	chats := worktree.NewChatTreeAncestryReader(lister, nil, nil)
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{
			"ws-a": {ID: "ws-a", Branch: "feature/a"},
		},
	}

	ws, err := worktree.Resolve(context.Background(), "chat-b", chats, workspaces)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if ws.ID != "ws-a" {
		t.Fatalf("resolved workspace = %q, want ws-a across two stacked folders", ws.ID)
	}
}

func TestResolve_AWorktreeOwningAncestorAcrossARealFolderIsFound(t *testing.T) {
	lister := &fakeChatLister{
		rows: []domain.Chat{
			{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
			{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-f"},
		},
	}
	folders := mocks.NewFolderStore()
	folders.Saved = []tree.Folder{{ID: "folder-f", Name: "notes"}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []tree.Node{
		{ID: "folder-f", Kind: tree.KindFolder, ParentID: "chat-a", Order: 0},
	}
	chats := worktree.NewChatTreeAncestryReader(lister, folders, nodes)
	workspaces := &fakeWorkspaceReader{
		byID: map[string]domain.Workspace{"ws-a": {ID: "ws-a", Branch: "feature/a"}},
	}

	ws, err := worktree.Resolve(context.Background(), "chat-b", chats, workspaces)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if ws.ID != "ws-a" {
		t.Fatalf("resolved workspace = %q, want ws-a (chat-b's nearest worktree-owning ancestor across the REAL folder-f)", ws.ID)
	}
}

func TestChatsForWorkspace_ARealFolderIsCrossedNotJustABakedInOne(t *testing.T) {
	lister := &fakeChatLister{
		rows: []domain.Chat{
			{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
			{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-f"},
		},
	}
	folders := mocks.NewFolderStore()
	folders.Saved = []tree.Folder{{ID: "folder-f", Name: "notes"}}
	nodes := mocks.NewNodePlacements()
	nodes.Rows = []tree.Node{
		{ID: "folder-f", Kind: tree.KindFolder, ParentID: "chat-a", Order: 0},
	}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, folders, nodes)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	want := []string{"chat-a", "chat-b"}
	if !slices.Equal(chatIDs, want) {
		t.Fatalf("chats = %v, want %v (chat-b found across the real folder-f)", chatIDs, want)
	}
}

func TestChatsForWorkspace_ReadsTheNodeTableOncePerCallNotOncePerChat(t *testing.T) {
	rows := []domain.Chat{{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"}}
	nodes := &countingNodes{NodePlacements: mocks.NewNodePlacements()}
	folders := mocks.NewFolderStore()
	for i := range 20 {
		id := "chat-" + string(rune('b'+i))
		rows = append(rows, domain.Chat{ID: id, Type: domain.ChatTypeChat, ParentID: "chat-a"})
	}

	if _, err := worktree.ChatsForWorkspace(
		context.Background(), "ws-a", &fakeChatLister{rows: rows}, folders, nodes,
	); err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if nodes.reads != 1 {
		t.Fatalf("node table reads = %d, want 1", nodes.reads)
	}
}

type countingNodes struct {
	*mocks.NodePlacements
	reads int
}

func (c *countingNodes) ListByParent(
	ctx context.Context,
	parentID string,
) ([]tree.Node, error) {
	c.reads++
	return c.NodePlacements.ListByParent(ctx, parentID)
}

func (c *countingNodes) ListAll(
	ctx context.Context,
) ([]tree.Node, error) {
	c.reads++
	return c.NodePlacements.ListAll(ctx)
}

func TestResolve_ChatListerErrorViaTheTreeAncestryReaderIsSurfacedWithContextNotSwallowed(t *testing.T) {
	cause := errors.New("chat lister unavailable")
	chats := worktree.NewChatTreeAncestryReader(&fakeChatLister{err: cause}, nil, nil)
	workspaces := &fakeWorkspaceReader{byID: map[string]domain.Workspace{}}

	ws, err := worktree.Resolve(context.Background(), "chat-b", chats, workspaces)
	if err == nil {
		t.Fatalf("Resolve returned nil error, want the ChatLister failure wrapped")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want it to wrap %v", err, cause)
	}
	if ws != (domain.Workspace{}) {
		t.Fatalf("workspace = %+v, want zero value alongside the error", ws)
	}
}

func sharedWorktreeForest() []domain.Chat {
	return []domain.Chat{
		{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "chat-a"},
		{ID: "folder-f", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
		{ID: "chat-c", Type: domain.ChatTypeChat, ParentID: "folder-f"},
		{ID: "folder-g", Type: domain.ChatTypeFolder, ParentID: "folder-f"},
		{ID: "chat-d", Type: domain.ChatTypeChat, ParentID: "folder-g"},
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-z", Type: domain.ChatTypeChat, WorkspaceID: "ws-z"},
		{ID: "chat-z-child", Type: domain.ChatTypeChat, ParentID: "chat-z"},
		{ID: "chat-loose", Type: domain.ChatTypeChat},
	}
}

func TestChatsForWorkspace_EverySiblingSharingOneWorktreeIsReturned(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	want := []string{"chat-a", "chat-b", "chat-c", "chat-d"}
	if !slices.Equal(chatIDs, want) {
		t.Fatalf("chats = %v, want %v", chatIDs, want)
	}
}

func TestChatsForWorkspace_AFolderIsNeverReturned(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if slices.Contains(chatIDs, "folder-f") || slices.Contains(chatIDs, "folder-g") {
		t.Fatalf("chats = %v, want no folder rows", chatIDs)
	}
}

func TestChatsForWorkspace_AnotherWorkspacesChatsAreExcluded(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-z", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	want := []string{"chat-z", "chat-z-child"}
	if !slices.Equal(chatIDs, want) {
		t.Fatalf("chats = %v, want %v", chatIDs, want)
	}
}

func TestChatsForWorkspace_AChildOwningItsOwnWorktreeShadowsItsParents(t *testing.T) {
	lister := &fakeChatLister{rows: []domain.Chat{
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-fork", Type: domain.ChatTypeChat, ParentID: "chat-a", WorkspaceID: "ws-fork"},
		{ID: "chat-fork-child", Type: domain.ChatTypeChat, ParentID: "chat-fork"},
	}}

	shared, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if !slices.Equal(shared, []string{"chat-a"}) {
		t.Fatalf("ws-a chats = %v, want only chat-a", shared)
	}
	forked, err := worktree.ChatsForWorkspace(context.Background(), "ws-fork", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if !slices.Equal(forked, []string{"chat-fork", "chat-fork-child"}) {
		t.Fatalf("ws-fork chats = %v, want chat-fork and chat-fork-child", forked)
	}
}

func TestChatsForWorkspace_AWorkspaceNobodyPointsAtIsEmptyNotAnError(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-nobody", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if len(chatIDs) != 0 {
		t.Fatalf("chats = %v, want none", chatIDs)
	}
}

func TestChatsForWorkspace_AnEmptyWorkspaceIDMatchesNothing(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if len(chatIDs) != 0 {
		t.Fatalf("chats = %v, want none", chatIDs)
	}
	if lister.calls != 0 {
		t.Fatalf("ListChats calls = %d, want 0 — an empty id needs no forest", lister.calls)
	}
}

func TestChatsForWorkspace_TheForestIsReadExactlyOncePerCall(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	if _, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil); err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("ListChats calls = %d, want exactly 1", lister.calls)
	}
}

func TestChatsForWorkspace_IsTheExactInverseOfResolve(t *testing.T) {
	rows := sharedWorktreeForest()
	lister := &fakeChatLister{rows: rows}
	chats := worktree.NewChatTreeAncestryReader(lister, nil, nil)
	workspaces := &fakeWorkspaceReader{byID: map[string]domain.Workspace{
		"ws-a": {ID: "ws-a"},
		"ws-z": {ID: "ws-z"},
	}}

	fanout, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	for _, row := range rows {
		if row.Type == domain.ChatTypeFolder {
			continue
		}
		ws, resolveErr := worktree.Resolve(context.Background(), row.ID, chats, workspaces)
		resolved := resolveErr == nil && ws.ID == "ws-a"
		if resolved != slices.Contains(fanout, row.ID) {
			t.Fatalf(
				"chat %s: Resolve→ws-a = %t but fan-out membership = %t (fan-out %v)",
				row.ID, resolved, slices.Contains(fanout, row.ID), fanout,
			)
		}
	}
}

func TestChatsForWorkspace_AParentCycleTerminates(t *testing.T) {
	lister := &fakeChatLister{rows: []domain.Chat{
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-x", Type: domain.ChatTypeChat, ParentID: "chat-y"},
		{ID: "chat-y", Type: domain.ChatTypeChat, ParentID: "chat-x"},
	}}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if !slices.Equal(chatIDs, []string{"chat-a"}) {
		t.Fatalf("chats = %v, want only chat-a", chatIDs)
	}
}

func TestChatsForWorkspace_ChatListerErrorIsSurfacedWithContextNotSwallowed(t *testing.T) {
	cause := errors.New("chat lister unavailable")
	lister := &fakeChatLister{err: cause}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister, nil, nil)
	if err == nil {
		t.Fatalf("ChatsForWorkspace returned nil error, want the ChatLister failure wrapped")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("err = %v, want it to wrap %v", err, cause)
	}
	if chatIDs != nil {
		t.Fatalf("chats = %v, want nil alongside the error", chatIDs)
	}
}

type fakeChatLister struct {
	rows  []domain.Chat
	err   error
	calls int
}

func (f *fakeChatLister) ListChats(
	_ context.Context,
) ([]domain.Chat, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.rows, nil
}

type fakeChatAncestryReader struct {
	ancestry map[string][]domain.Chat
	err      error
}

func (f *fakeChatAncestryReader) Ancestors(
	_ context.Context,
	chatID string,
) ([]domain.Chat, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.ancestry[chatID], nil
}

type fakeWorkspaceReader struct {
	byID map[string]domain.Workspace
	err  error
	gets []string
}

func (f *fakeWorkspaceReader) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	f.gets = append(f.gets, id)
	if f.err != nil {
		return domain.Workspace{}, f.err
	}
	return f.byID[id], nil
}
