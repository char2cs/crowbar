package project_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeOwningChats struct {
	mu sync.Mutex

	nextID    int
	minted    []string
	attached  map[string]string
	discarded []string

	mintErr   error
	attachErr error
}

func newFakeOwningChats() *fakeOwningChats {
	return &fakeOwningChats{attached: map[string]string{}}
}

func (f *fakeOwningChats) MintOwningChat(
	_ context.Context,
	_ string,
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

func newImportUsecase(
	t *testing.T,
	deps project.ImportDeps,
) project.ImportUsecase {
	t.Helper()
	if deps.Nodes == nil {
		deps.Nodes = mocks.NewNodePlacements()
	}
	if deps.CrowbarHome == nil {
		deps.CrowbarHome = tempCrowbarHome(t)
	}
	uc := project.NewImport(deps)
	uc.SetOwningChats(newFakeOwningChats())
	return uc
}

func tempCrowbarHome(t *testing.T) func() (string, error) {
	t.Helper()
	dir := t.TempDir()
	return func() (string, error) { return dir, nil }
}
