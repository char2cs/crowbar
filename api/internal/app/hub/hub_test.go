package hub_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type fakeSubscriber struct {
	projects    []dto.ProjectDTO
	repos       []dto.RepoDTO
	workspaces  []dto.WorkspaceDTO
	threads     []dto.ThreadDTO
	terminals   []dto.TerminalSessionDTO
	gitStatuses []gitdomain.GitStatus
	fileEvents  []domain.FileChangeEvent
	agentChats  []agentChatPush
}

type agentChatPush struct {
	chatID      string
	workspaceID string
	kind        string
}

func (f *fakeSubscriber) PushProject(
	p dto.ProjectDTO,
) {
	f.projects = append(f.projects, p)
}

func (f *fakeSubscriber) PushRepo(
	r dto.RepoDTO,
) {
	f.repos = append(f.repos, r)
}

func (f *fakeSubscriber) PushWorkspace(
	w dto.WorkspaceDTO,
) {
	f.workspaces = append(f.workspaces, w)
}

func (f *fakeSubscriber) PushThread(
	t dto.ThreadDTO,
) {
	f.threads = append(f.threads, t)
}

func (f *fakeSubscriber) PushTerminalSession(
	s dto.TerminalSessionDTO,
) {
	f.terminals = append(f.terminals, s)
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

func (f *fakeSubscriber) PushAgentChat(
	chatID string,
	workspaceID string,
	kind string,
) {
	f.agentChats = append(f.agentChats, agentChatPush{chatID: chatID, workspaceID: workspaceID, kind: kind})
}

func TestHub_BroadcastProject_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastProject(dto.ProjectDTO{ID: "p1"})

	assert.Len(t, a.projects, 1)
	assert.Len(t, b.projects, 1)
	assert.Equal(t, "p1", a.projects[0].ID)
}

func TestHub_BroadcastRepo_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastRepo(dto.RepoDTO{ID: "r1", ProjectID: "p1"})

	assert.Len(t, a.repos, 1)
	assert.Len(t, b.repos, 1)
	assert.Equal(t, "r1", a.repos[0].ID)
}

func TestHub_BroadcastWorkspace_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastWorkspace(dto.WorkspaceDTO{ID: "w1", RepoID: "r1", ProjectID: "p1"})

	assert.Len(t, a.workspaces, 1)
	assert.Len(t, b.workspaces, 1)
	assert.Equal(t, "w1", a.workspaces[0].ID)
}

func TestHub_BroadcastThread_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastThread(dto.ThreadDTO{ID: "t1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"})

	assert.Len(t, a.threads, 1)
	assert.Len(t, b.threads, 1)
	assert.Equal(t, "t1", a.threads[0].ID)
}

func TestHub_BroadcastTerminalSession_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastTerminalSession(dto.TerminalSessionDTO{ID: "s1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"})

	assert.Len(t, a.terminals, 1)
	assert.Len(t, b.terminals, 1)
	assert.Equal(t, "s1", a.terminals[0].ID)
}

func TestHub_BroadcastGit_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastGit("w1", gitdomain.GitStatus{Branch: "main"})

	assert.Len(t, a.gitStatuses, 1)
	assert.Len(t, b.gitStatuses, 1)
	assert.Equal(t, "main", a.gitStatuses[0].Branch)
}

func TestHub_BroadcastFile_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastFile(domain.FileChangeEvent{WsID: "w1", Path: "a.go"})

	assert.Len(t, a.fileEvents, 1)
	assert.Len(t, b.fileEvents, 1)
	assert.Equal(t, "a.go", a.fileEvents[0].Path)
}

func TestHub_BroadcastAgentChat_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastAgentChat("c1", "w1", "bound")

	assert.Len(t, a.agentChats, 1)
	assert.Len(t, b.agentChats, 1)
	assert.Equal(t, agentChatPush{chatID: "c1", workspaceID: "w1", kind: "bound"}, a.agentChats[0])
}

func TestHub_NoSubscribers_DoesNotPanic(t *testing.T) {
	h := hub.NewHub()
	assert.NotPanics(t, func() {
		h.BroadcastWorkspace(dto.WorkspaceDTO{ID: "w1"})
	})
}

func TestHub_ImplementsWebSocketHub(t *testing.T) {
	var _ hub.WebSocketHub = hub.NewHub()
}
