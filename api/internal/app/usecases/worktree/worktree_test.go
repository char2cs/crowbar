package worktree_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
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

// TestResolve_AWorktreeOwningAncestorAcrossAFolderIsFound is the regression
// test for the bug in commit 8f7ca472: chat-a owns a worktree, folder-f is
// filed under chat-a, and chat-b is filed under folder-f. Resolving chat-b
// must find chat-a's workspace by walking through folder-f, not stop at it.
//
// It goes through NewChatTreeAncestryReader — the real production adapter,
// not a fake standing in for its answer — over a fakeChatLister that supplies
// nothing but raw rows and real ParentID edges. Whether folder-f is crossed
// is entirely up to the tree walk under test.
func TestResolve_AWorktreeOwningAncestorAcrossAFolderIsFound(t *testing.T) {
	lister := &fakeChatLister{
		rows: []domain.Chat{
			{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-f"},
			{ID: "chat-a", Type: domain.ChatTypeChat, ParentID: "", WorkspaceID: "ws-a"},
			{ID: "folder-f", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
		},
	}
	chats := worktree.NewChatTreeAncestryReader(lister)
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

// TestResolve_AWorktreeOwningAncestorAcrossNestedFoldersIsFound strengthens
// the case above with two folders stacked between the chat and its
// worktree-owning ancestor, so the walk has to actually climb rather than
// resolve on a lucky single hop.
func TestResolve_AWorktreeOwningAncestorAcrossNestedFoldersIsFound(t *testing.T) {
	lister := &fakeChatLister{
		rows: []domain.Chat{
			{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
			{ID: "folder-outer", Type: domain.ChatTypeFolder, ParentID: "chat-a"},
			{ID: "folder-inner", Type: domain.ChatTypeFolder, ParentID: "folder-outer"},
			{ID: "chat-b", Type: domain.ChatTypeChat, ParentID: "folder-inner"},
		},
	}
	chats := worktree.NewChatTreeAncestryReader(lister)
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

// TestResolve_ChatListerErrorViaTheTreeAncestryReaderIsSurfacedWithContextNotSwallowed
// mirrors TestResolve_ChatAncestryReaderErrorIsSurfacedWithContextNotSwallowed
// for the new adapter's own failure path.
func TestResolve_ChatListerErrorViaTheTreeAncestryReaderIsSurfacedWithContextNotSwallowed(t *testing.T) {
	cause := errors.New("chat lister unavailable")
	chats := worktree.NewChatTreeAncestryReader(&fakeChatLister{err: cause})
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

// sharedWorktreeForest is the normal shape Step 1 made common: chat-a owns
// ws-a, and a batch of siblings hangs off it — one direct child, one across a
// folder, one across two stacked folders. chat-z owns an unrelated ws-z with a
// child of its own, and chat-loose floats at the root owning no worktree
// anywhere in its ancestry.
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

// TestChatsForWorkspace_EverySiblingSharingOneWorktreeIsReturned is the whole
// point of the inverse: a write through ANY of these chats' routes is a write
// to one worktree, so all four must be told about it — the owner, the direct
// child, and the two filed under folders.
func TestChatsForWorkspace_EverySiblingSharingOneWorktreeIsReturned(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	want := []string{"chat-a", "chat-b", "chat-c", "chat-d"}
	if !slices.Equal(chatIDs, want) {
		t.Fatalf("chats = %v, want %v", chatIDs, want)
	}
}

// TestChatsForWorkspace_AFolderIsNeverReturned pins that folder-f and folder-g
// are crossed but not counted: a folder holds chats, it is not one, and nothing
// subscribes under a folder id.
func TestChatsForWorkspace_AFolderIsNeverReturned(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if slices.Contains(chatIDs, "folder-f") || slices.Contains(chatIDs, "folder-g") {
		t.Fatalf("chats = %v, want no folder rows", chatIDs)
	}
}

// TestChatsForWorkspace_AnotherWorkspacesChatsAreExcluded proves the answer is
// scoped, not "every chat this daemon knows": chat-z owns its own worktree and
// its child inherits that one, so neither belongs on ws-a's fan-out.
func TestChatsForWorkspace_AnotherWorkspacesChatsAreExcluded(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-z", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	want := []string{"chat-z", "chat-z-child"}
	if !slices.Equal(chatIDs, want) {
		t.Fatalf("chats = %v, want %v", chatIDs, want)
	}
}

// TestChatsForWorkspace_AChildOwningItsOwnWorktreeShadowsItsParents is the
// case a naive "every descendant of the owner" walk gets wrong: chat-fork hangs
// off chat-a but owns ws-fork, so it reads and writes a DIFFERENT worktree and
// must not be told about ws-a's writes. Its own child inherits the fork, not
// the parent.
func TestChatsForWorkspace_AChildOwningItsOwnWorktreeShadowsItsParents(t *testing.T) {
	lister := &fakeChatLister{rows: []domain.Chat{
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-fork", Type: domain.ChatTypeChat, ParentID: "chat-a", WorkspaceID: "ws-fork"},
		{ID: "chat-fork-child", Type: domain.ChatTypeChat, ParentID: "chat-fork"},
	}}

	shared, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if !slices.Equal(shared, []string{"chat-a"}) {
		t.Fatalf("ws-a chats = %v, want only chat-a", shared)
	}
	forked, err := worktree.ChatsForWorkspace(context.Background(), "ws-fork", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if !slices.Equal(forked, []string{"chat-fork", "chat-fork-child"}) {
		t.Fatalf("ws-fork chats = %v, want chat-fork and chat-fork-child", forked)
	}
}

// TestChatsForWorkspace_AWorkspaceNobodyPointsAtIsEmptyNotAnError: having no
// subscribers is a fact, not a failure — a push for it simply reaches nobody.
func TestChatsForWorkspace_AWorkspaceNobodyPointsAtIsEmptyNotAnError(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-nobody", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if len(chatIDs) != 0 {
		t.Fatalf("chats = %v, want none", chatIDs)
	}
}

// TestChatsForWorkspace_AnEmptyWorkspaceIDMatchesNothing guards the reading
// that would be catastrophic: chat-loose resolves to NO workspace, and its
// resolved workspace id is "" — so an empty query must not be answered with
// "every chat whose ancestry owns no worktree".
func TestChatsForWorkspace_AnEmptyWorkspaceIDMatchesNothing(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "", lister)
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

// TestChatsForWorkspace_TheForestIsReadExactlyOncePerCall pins the shape, not
// just the answer: resolving each chat in turn through Resolve would re-list
// the entire forest per chat, which is the same answer at N times the cost.
func TestChatsForWorkspace_TheForestIsReadExactlyOncePerCall(t *testing.T) {
	lister := &fakeChatLister{rows: sharedWorktreeForest()}

	if _, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister); err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if lister.calls != 1 {
		t.Fatalf("ListChats calls = %d, want exactly 1", lister.calls)
	}
}

// TestChatsForWorkspace_IsTheExactInverseOfResolve cross-checks the two
// directions against each other over one forest: every chat the fan-out set
// names must Resolve to that workspace, and every chat it omits must not.
func TestChatsForWorkspace_IsTheExactInverseOfResolve(t *testing.T) {
	rows := sharedWorktreeForest()
	lister := &fakeChatLister{rows: rows}
	chats := worktree.NewChatTreeAncestryReader(lister)
	workspaces := &fakeWorkspaceReader{byID: map[string]domain.Workspace{
		"ws-a": {ID: "ws-a"},
		"ws-z": {ID: "ws-z"},
	}}

	fanout, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister)
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

// TestChatsForWorkspace_AParentCycleTerminates: a row whose parent chain loops
// back on itself is corrupt data, not a reason to hang the pushing goroutine.
func TestChatsForWorkspace_AParentCycleTerminates(t *testing.T) {
	lister := &fakeChatLister{rows: []domain.Chat{
		{ID: "chat-a", Type: domain.ChatTypeChat, WorkspaceID: "ws-a"},
		{ID: "chat-x", Type: domain.ChatTypeChat, ParentID: "chat-y"},
		{ID: "chat-y", Type: domain.ChatTypeChat, ParentID: "chat-x"},
	}}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister)
	if err != nil {
		t.Fatalf("ChatsForWorkspace returned error: %v", err)
	}
	if !slices.Equal(chatIDs, []string{"chat-a"}) {
		t.Fatalf("chats = %v, want only chat-a", chatIDs)
	}
}

// TestChatsForWorkspace_ChatListerErrorIsSurfacedWithContextNotSwallowed
// mirrors Resolve's own failure-path test: a fan-out that cannot be computed is
// never silently an empty one, which would look exactly like "no siblings".
func TestChatsForWorkspace_ChatListerErrorIsSurfacedWithContextNotSwallowed(t *testing.T) {
	cause := errors.New("chat lister unavailable")
	lister := &fakeChatLister{err: cause}

	chatIDs, err := worktree.ChatsForWorkspace(context.Background(), "ws-a", lister)
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

// fakeChatLister stands in for the container's real ChatLister
// (usecases/chat.Usecase.ListChats): a fixed, unordered set of raw chat rows
// carrying real ParentID edges, or a fixed error every call returns instead.
// It bakes in no folder-crossing or ancestry logic of its own — that is
// exactly what NewChatTreeAncestryReader, the real adapter under test, is
// left to compute from these rows.
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

// fakeChatAncestryReader stands in for the container's real adapter: a fixed
// ancestry per chat id, or a fixed error every call returns instead.
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

// fakeWorkspaceReader stands in for the real workspace usecase: a fixed row
// per id, or a fixed error every call returns instead. It also records every
// id it was asked for, so a test can prove Resolve short-circuits and never
// looks up a workspace it didn't need.
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
