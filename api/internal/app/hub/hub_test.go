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
	folders     []dto.FolderDTO
	workspaces  []dto.WorkspaceDTO
	threads     []dto.ThreadDTO
	terminals   []dto.TerminalSessionDTO
	gitStatuses []gitdomain.GitStatus
	fileEvents  []domain.FileChangeEvent
	agentChats  []agentChatPush
	agentRunner []agentRunnerPush

	agentChatFolders []agentChatFolderPush
	agentChatWaits   []agentChatWaitPush
	agentCompactions []agentCompactionPush
}

type agentCompactionPush struct {
	chatID      string
	workspaceID string
	active      bool
}

type agentChatWaitPush struct {
	chatID      string
	workspaceID string
	wait        *dto.AgentTerminalWaitDTO
}

type agentChatFolderPush struct {
	folderID    string
	workspaceID string
	kind        string
}

type agentChatPush struct {
	chatID      string
	workspaceID string
	kind        string
	working     bool
}

type agentRunnerPush struct {
	runnerID    string
	workspaceID string
	chatID      string
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

func (f *fakeSubscriber) PushFolder(
	fd dto.FolderDTO,
) {
	f.folders = append(f.folders, fd)
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
	working bool,
) {
	f.agentChats = append(f.agentChats, agentChatPush{
		chatID:      chatID,
		workspaceID: workspaceID,
		kind:        kind,
		working:     working,
	})
}

func (f *fakeSubscriber) PushAgentChatTerminalWait(
	chatID string,
	workspaceID string,
	wait *dto.AgentTerminalWaitDTO,
) {
	f.agentChatWaits = append(f.agentChatWaits, agentChatWaitPush{
		chatID:      chatID,
		workspaceID: workspaceID,
		wait:        wait,
	})
}

func (f *fakeSubscriber) PushAgentChatPromptSettled(_, _, _ string)   {}
func (f *fakeSubscriber) PushAgentChatMessageDelta(_, _, _, _ string) {}

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

func (f *fakeSubscriber) PushAgentRunner(
	runnerID string,
	workspaceID string,
	chatID string,
	kind string,
) {
	f.agentRunner = append(f.agentRunner, agentRunnerPush{
		runnerID: runnerID, workspaceID: workspaceID, chatID: chatID, kind: kind,
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

func TestHub_BroadcastFolder_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastFolder(dto.FolderDTO{ID: "f1", RepoID: "r1", ProjectID: "p1"})

	assert.Len(t, a.folders, 1)
	assert.Len(t, b.folders, 1)
	assert.Equal(t, "f1", a.folders[0].ID)
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

	h.BroadcastAgentChat("c1", "w1", "bound", true)

	assert.Len(t, a.agentChats, 1)
	assert.Len(t, b.agentChats, 1)
	assert.Equal(t,
		agentChatPush{chatID: "c1", workspaceID: "w1", kind: "bound", working: true},
		a.agentChats[0])
}

// TestHub_BroadcastAgentChatTerminalWait_FansOut proves the terminal-wait edge
// reaches every registered subscriber with the chat id, workspace id and payload
// intact — the same fan-out contract every other Broadcast* method on the hub
// has, since this is the frame that puts a "waiting in the terminal" banner up.
func TestHub_BroadcastAgentChatTerminalWait_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	wait := &dto.AgentTerminalWaitDTO{Kind: domain.AgentTerminalWaitTrust}
	h.BroadcastAgentChatTerminalWait("c1", "w1", wait)

	assert.Len(t, a.agentChatWaits, 1)
	assert.Len(t, b.agentChatWaits, 1)
	assert.Equal(t,
		agentChatWaitPush{chatID: "c1", workspaceID: "w1", wait: wait},
		a.agentChatWaits[0])
}

// TestHub_BroadcastAgentChatTerminalWait_ClearingEdgeReachesSubscribers proves a
// nil payload fans out exactly like a populated one. This is the frame that TAKES
// A BANNER DOWN: a broadcast that dropped a nil wait on the way to a subscriber
// would strand a client showing that banner over a chat that is fine again.
func TestHub_BroadcastAgentChatTerminalWait_ClearingEdgeReachesSubscribers(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	h.Register(a)

	h.BroadcastAgentChatTerminalWait("c1", "w1", nil)

	assert.Len(t, a.agentChatWaits, 1)
	assert.Equal(t,
		agentChatWaitPush{chatID: "c1", workspaceID: "w1", wait: nil},
		a.agentChatWaits[0])
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

// TestHub_BroadcastAgentRunner_FansOut pins the runner frame's shape: it carries
// the CHAT the runner is pointed at as of the event, so a `moved` frame names the
// chat the CLI moved INTO and a client can re-point the tab following that runner.
func TestHub_BroadcastAgentRunner_FansOut(t *testing.T) {
	h := hub.NewHub()
	a := &fakeSubscriber{}
	b := &fakeSubscriber{}
	h.Register(a)
	h.Register(b)

	h.BroadcastAgentRunner("r1", "w1", "chat-b", "moved")

	assert.Len(t, a.agentRunner, 1)
	assert.Len(t, b.agentRunner, 1)
	assert.Equal(t,
		agentRunnerPush{runnerID: "r1", workspaceID: "w1", chatID: "chat-b", kind: "moved"},
		a.agentRunner[0])
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
