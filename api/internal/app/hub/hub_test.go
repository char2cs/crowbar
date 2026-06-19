package hub_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type fakeSubscriber struct {
	workspaces  []domain.Workspace
	gitStatuses []gitdomain.GitStatus
	fileEvents  []domain.FileChangeEvent
}

func (f *fakeSubscriber) PushWorkspace(
	ws domain.Workspace,
) {
	f.workspaces = append(f.workspaces, ws)
}

func (f *fakeSubscriber) PushGit(
	_ string,
	status gitdomain.GitStatus,
) {
	f.gitStatuses = append(f.gitStatuses, status)
}

func (f *fakeSubscriber) PushFile(
	evt domain.FileChangeEvent,
) {
	f.fileEvents = append(f.fileEvents, evt)
}

func TestHub_BroadcastWorkspace_ReachesSubscribers(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastWorkspace(domain.Workspace{ID: "w1"})

	assert.Len(t, a.workspaces, 1)
	assert.Len(t, b.workspaces, 1)
	assert.Equal(t, "w1", a.workspaces[0].ID)
}

func TestHub_NoSubscribers_DoesNotPanic(t *testing.T) {
	h := hub.NewHub()
	assert.NotPanics(t, func() {
		h.BroadcastWorkspace(domain.Workspace{ID: "w1"})
	})
}
