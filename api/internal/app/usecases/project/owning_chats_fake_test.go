package project_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// fakeOwningChats stands in for the chat side every workspace a repo import is
// now born under. Unlike the hierarchy's own fake it delegates nothing: this
// usecase builds its workspace rows itself, so all the chat side does here is
// mint, attach and discard — which is exactly what this records.
type fakeOwningChats struct {
	mu sync.Mutex

	nextID    int
	minted    []string
	attached  map[string]string
	discarded []string

	// mintErr fails MintOwningChat and attachErr fails AttachOwningWorkspace, so
	// a test can prove that a workspace is never left behind when the chat that
	// must own it cannot be minted or cannot be pointed at it.
	mintErr   error
	attachErr error
}

func newFakeOwningChats() *fakeOwningChats {
	return &fakeOwningChats{attached: map[string]string{}}
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
// did at all.
func (f *fakeOwningChats) ownerOf(
	workspaceID string,
) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	chatID, ok := f.attached[workspaceID]
	return chatID, ok
}

func (f *fakeOwningChats) mintedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.minted)
}

func (f *fakeOwningChats) discards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.discarded...)
}

// newImportUsecase builds the import usecase already wired to a fresh owning-chat
// fake, which every construction in these tests needs now that no workspace can
// be created without one. A test that wants to assert on the chat side builds
// its own fake and calls SetOwningChats itself.
func newImportUsecase(
	deps project.ImportDeps,
) project.ImportUsecase {
	uc := project.NewImport(deps)
	uc.SetOwningChats(newFakeOwningChats())
	return uc
}
