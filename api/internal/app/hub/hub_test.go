package hub_test

import (
	"testing"

	agents "github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
)

type fakeSubscriber struct {
	projects        []dto.ProjectDTO
	repos           []dto.RepoDTO
	workspaces      []dto.WorkspaceDTO
	agentChatEvents []dto.AgentChatEvent

	agentChatFolders []agentChatFolderPush
}

type agentChatFolderPush struct {
	folderID    string
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

func (f *fakeSubscriber) PushAgentChatTelemetry(_, _ string, _ agents.Telemetry) {}
func (f *fakeSubscriber) PushAgentChatEvent(ev dto.AgentChatEvent) {
	f.agentChatEvents = append(f.agentChatEvents, ev)
}

func (f *fakeSubscriber) PushAgentChatFolder(
	folderID string,
	workspaceID string,
	kind string,
) {
	f.agentChatFolders = append(f.agentChatFolders, agentChatFolderPush{
		folderID:    folderID,
		workspaceID: workspaceID,
		kind:        kind,
	})
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

// TestHub_BroadcastAgentChatFolder_FansOut proves a chat-folder lifecycle
// event reaches every registered subscriber on the same workspace-scoped feed
// as BroadcastAgentChat — a chat folder is a plain GORM row with no aggregate
// projection to ride, so nothing else pushes this fact to subscribers.
func TestHub_BroadcastAgentChatFolder_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastAgentChatFolder("f1", "w1", "folder_created")

	want := []agentChatFolderPush{{folderID: "f1", workspaceID: "w1", kind: "folder_created"}}
	assert.Equal(t, want, a.agentChatFolders)
	assert.Equal(t, want, b.agentChatFolders)
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

// A chat snapshot frame reaches every subscriber whole: the chat DTO and its
// version travel together, which is what lets a client order them.
func TestHub_BroadcastAgentChatEvent_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	ev := dto.AgentChatEvent{
		ChatID: "c1", WorkspaceID: "w1", Kind: "moved", RunnerID: "r1", Version: 7,
		Chat: &dto.AgentChatDTO{ID: "c1", Version: 7, Phase: "live", LiveRunnerID: "r1"},
	}
	h.BroadcastAgentChatEvent(ev)

	assert.Len(t, a.agentChatEvents, 1)
	assert.Len(t, b.agentChatEvents, 1)
	assert.Equal(t, ev, a.agentChatEvents[0])
}
