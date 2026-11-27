package hierarchy_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeOwningChats struct {
	mu sync.Mutex
	uc hierarchy.Usecase

	nextID    int
	minted    []string
	attached  map[string]string
	discarded []string

	mintErr   error
	attachErr error
}

func newFakeOwningChats(uc hierarchy.Usecase) *fakeOwningChats {
	return &fakeOwningChats{uc: uc, attached: map[string]string{}}
}

func (f *fakeOwningChats) ImportBranchAsChat(
	ctx context.Context,
	in hierarchy.ImportedBranch,
) (string, string, error) {
	chatID, err := f.MintOwningChat(ctx, in.ChatID, in.ParentWorkspaceID)
	if err != nil {
		return "", "", err
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
		return chatID, "", err
	}
	if aErr := f.AttachOwningWorkspace(ctx, chatID, ws); aErr != nil {
		return chatID, "", aErr
	}
	return chatID, ws.ID, nil
}

func (f *fakeOwningChats) MintOwningChat(
	_ context.Context,
	chatID string,
	_ string,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mintErr != nil {
		return "", f.mintErr
	}
	if chatID == "" {
		f.nextID++
		chatID = fmt.Sprintf("chat-%d", f.nextID)
	}
	f.minted = append(f.minted, chatID)
	return chatID, nil
}

func (f *fakeOwningChats) OwnerOf(
	_ context.Context,
	workspaceID string,
) (string, error) {
	chatID, _ := f.ownerOf(workspaceID)
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

func withOwningChats(
	uc hierarchy.Usecase,
) *fakeOwningChats {
	chats := newFakeOwningChats(uc)
	uc.SetOwningChats(chats)
	return chats
}
