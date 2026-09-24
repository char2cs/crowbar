package hub_test

import (
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type fakeSubscriber struct {
	projects        []dto.ProjectDTO
	repos           []dto.RepoDTO
	workspaces      []dto.WorkspaceDTO
	threads         []dto.ThreadDTO
	terminals       []dto.TerminalSessionDTO
	gitStatuses     []gitdomain.GitStatus
	fileEvents      []domain.FileChangeEvent
	agentChatEvents []dto.AgentChatEvent

	agentChatFolders []agentChatFolderPush
	agentCompactions []agentCompactionPush
	promptSettled    []promptSettledPush
	messageDeltas    []messageDeltaPush
}

type promptSettledPush struct {
	chatID      string
	workspaceID string
	requestID   string
	consumed    bool
}

type messageDeltaPush struct {
	chatID      string
	workspaceID string
	messageID   string
	text        string
}

type agentCompactionPush struct {
	chatID      string
	workspaceID string
	active      bool
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

func (f *fakeSubscriber) PushAgentChatPromptSettled(
	chatID string,
	workspaceID string,
	requestID string,
	consumed bool,
) {
	f.promptSettled = append(f.promptSettled, promptSettledPush{
		chatID: chatID, workspaceID: workspaceID, requestID: requestID, consumed: consumed,
	})
}

func (f *fakeSubscriber) PushAgentChatMessageDelta(
	chatID string,
	workspaceID string,
	messageID string,
	text string,
	_ string,
) {
	f.messageDeltas = append(f.messageDeltas, messageDeltaPush{
		chatID: chatID, workspaceID: workspaceID, messageID: messageID, text: text,
	})
}

func (f *fakeSubscriber) PushAgentChatPlan(_, _ string, _ []agents.PlanStep)     {}
func (f *fakeSubscriber) PushAgentChatTelemetry(_, _ string, _ agents.Telemetry) {}
func (f *fakeSubscriber) PushAgentChatEvent(ev dto.AgentChatEvent) {
	f.agentChatEvents = append(f.agentChatEvents, ev)
}

func (f *fakeSubscriber) PushAgentChatCompaction(
	chatID string,
	workspaceID string,
	active bool,
) {
	f.agentCompactions = append(f.agentCompactions, agentCompactionPush{
		chatID: chatID, workspaceID: workspaceID, active: active,
	})
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

	h.BroadcastTerminalSession(dto.TerminalSessionDTO{ID: "s1", ChatID: "chat-1"})

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

// TestHub_BroadcastAgentChatCompaction_FansOut proves the live compaction edge
// reaches every registered subscriber intact, both ways round — this is the
// frame the "Compacting…" indicator has to key off, since the ledger's own
// interruption record for a compaction is born already resolved and can never
// drive it (see the doc comment on BroadcastAgentChatCompaction).
func TestHub_BroadcastAgentChatCompaction_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastAgentChatCompaction("c1", "w1", true)
	h.BroadcastAgentChatCompaction("c1", "w1", false)

	want := []agentCompactionPush{
		{chatID: "c1", workspaceID: "w1", active: true},
		{chatID: "c1", workspaceID: "w1", active: false},
	}
	assert.Equal(t, want, a.agentCompactions)
	assert.Equal(t, want, b.agentCompactions)
}

// TestHub_BroadcastAgentChatPromptSettled_FansOut proves the "prompt retired
// without ever opening a turn" edge reaches every registered subscriber with
// the chat/workspace/request ids intact — this is the frame that clears a
// pending "waiting on you" affordance when a runner picks a prompt up without
// ever producing a turn for it.
func TestHub_BroadcastAgentChatPromptSettled_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastAgentChatPromptSettled("c1", "w1", "req-1", true)

	want := []promptSettledPush{
		{chatID: "c1", workspaceID: "w1", requestID: "req-1", consumed: true},
	}
	assert.Equal(t, want, a.promptSettled)
	assert.Equal(t, want, b.promptSettled)
}

// TestHub_BroadcastAgentChatMessageDelta_FansOut proves the growing-message
// edge reaches every registered subscriber with the chat/workspace/message ids
// and the delta text intact. This broadcast is the highest-frequency one on the
// hub (roughly 1.4/s per streaming chat) and deliberately never touches durable
// storage, so a dropped fan-out here is invisible anywhere but the live client.
func TestHub_BroadcastAgentChatMessageDelta_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastAgentChatMessageDelta("c1", "w1", "m1", "partial tex", "")
	h.BroadcastAgentChatMessageDelta("c1", "w1", "m1", "partial text", "")

	want := []messageDeltaPush{
		{chatID: "c1", workspaceID: "w1", messageID: "m1", text: "partial tex"},
		{chatID: "c1", workspaceID: "w1", messageID: "m1", text: "partial text"},
	}
	assert.Equal(t, want, a.messageDeltas)
	assert.Equal(t, want, b.messageDeltas)
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
