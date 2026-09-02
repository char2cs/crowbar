package worktree_test

import (
	"context"
	"errors"
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

// fakeChatLister stands in for the container's real ChatLister
// (usecases/chat.Usecase.ListChats): a fixed, unordered set of raw chat rows
// carrying real ParentID edges, or a fixed error every call returns instead.
// It bakes in no folder-crossing or ancestry logic of its own — that is
// exactly what NewChatTreeAncestryReader, the real adapter under test, is
// left to compute from these rows.
type fakeChatLister struct {
	rows []domain.Chat
	err  error
}

func (f *fakeChatLister) ListChats(
	_ context.Context,
) ([]domain.Chat, error) {
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
