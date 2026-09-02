package hierarchy_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeOwningChats stands in for the chat side every workspace this usecase
// creates is now born under, and it is deliberately not a null object: it
// delegates the actual workspace create back through CreateChild exactly as the
// container's real adapter delegates it through the chat usecase.
//
// That fidelity is the point. These are regression tests about GIT — a real
// worktree on disk, a real placeholder, a real PR chain — and a fake that
// merely returned a fabricated id would let every one of them pass while
// creating nothing. Delegating keeps the git half honest and adds only the half
// this change introduces: an owning chat minted before the workspace, and taken
// back out when the workspace never arrives.
type fakeOwningChats struct {
	mu sync.Mutex
	uc hierarchy.Usecase

	nextID    int
	minted    []string
	attached  map[string]string
	discarded []string

	// mintErr fails MintOwningChat, so a test can prove nothing is created when
	// the chat cannot be minted first.
	mintErr error
	// attachErr fails AttachOwningWorkspace, the double fault that would
	// otherwise leave a workspace nothing owns.
	attachErr error
}

func newFakeOwningChats(uc hierarchy.Usecase) *fakeOwningChats {
	return &fakeOwningChats{uc: uc, attached: map[string]string{}}
}

// ImportBranchAsChat mirrors production: mint the chat, create the workspace,
// attach the two, discard the chat if the create failed.
func (f *fakeOwningChats) ImportBranchAsChat(
	ctx context.Context,
	in hierarchy.ImportedBranch,
) (string, error) {
	chatID, err := f.MintOwningChat(ctx, in.ParentWorkspaceID)
	if err != nil {
		return "", err
	}
	ownWorktree := true
	ws, err := f.uc.CreateChild(ctx, hierarchy.CreateChildInput{
		RepoID:       in.RepoID,
		ProjectID:    in.ProjectID,
		RepoPath:     in.RepoPath,
		RemoteURL:    in.RemoteURL,
		Branch:       in.Branch,
		ParentID:     in.ParentWorkspaceID,
		ParentBranch: in.ParentBranch,
		ForceLocked:  in.ForceLocked,
		OwnWorktree:  &ownWorktree,
	})
	if err != nil {
		_ = f.DiscardOwningChat(ctx, chatID)
		return "", err
	}
	if aErr := f.AttachOwningWorkspace(ctx, chatID, ws); aErr != nil {
		_ = f.DiscardOwningChat(ctx, chatID)
		return "", aErr
	}
	return ws.ID, nil
}

func (f *fakeOwningChats) MintOwningChat(
	_ context.Context,
	_ string,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mintErr != nil {
		return "", f.mintErr
	}
	f.nextID++
	chatID := fmt.Sprintf("chat-%d", f.nextID)
	f.minted = append(f.minted, chatID)
	return chatID, nil
}

func (f *fakeOwningChats) AttachOwningWorkspace(
	_ context.Context,
	chatID string,
	ws domain.Workspace,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.attachErr != nil {
		return f.attachErr
	}
	f.attached[ws.ID] = chatID
	return nil
}

func (f *fakeOwningChats) DiscardOwningChat(
	_ context.Context,
	chatID string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discarded = append(f.discarded, chatID)
	return nil
}

// ownerOf reports the chat that ended up owning a workspace, and whether one
// did at all — the question every test about this change is really asking.
func (f *fakeOwningChats) ownerOf(
	workspaceID string,
) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	chatID, ok := f.attached[workspaceID]
	return chatID, ok
}

func (f *fakeOwningChats) discards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.discarded...)
}

// withOwningChats wires a usecase to a fresh fake and hands both back, so a
// test that only needs the import to work says so in one line.
func withOwningChats(
	uc hierarchy.Usecase,
) *fakeOwningChats {
	chats := newFakeOwningChats(uc)
	uc.SetOwningChats(chats)
	return chats
}
