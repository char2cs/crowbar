package v0

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// seedHomeWorkspace creates a real PROJECT-HOME workspace: a row with a project
// and NO repo. It is the shape the whole leak turned on — a home chat's frames
// legitimately carry an empty RepoID, which the repoId filter reads as "this
// row has no repo to be held to" — so a fixture that faked it with an ordinary
// workspace would prove nothing.
func seedHomeWorkspace(
	t *testing.T,
	a *app.Container,
	id string,
	projectID string,
) {
	t.Helper()
	_, err := a.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{ID: id, ProjectID: projectID, Kind: domain.WorkspaceKindHome},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	a.Repositories.WaitQuiescent()
	got, err := a.Repositories.Workspace.Get(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, projectID, got.ProjectID)
	require.Empty(t, got.RepoID,
		"a project-home workspace owns no repo — that is what makes its chat frames repo-less")
}

// serveChats mounts the agent-chat broadcaster on the repo-scoped shape the
// live routes carry (.../projects/:projectId/repos/:repoId/chats/ws), directly
// rather than through Register: these tests are about which client a frame
// reaches, not about the group guards in front of the upgrade.
func serveChats(
	t *testing.T,
	a *app.Container,
) (*Container, *httptest.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := New(a, nil)
	r := gin.New()
	r.GET(
		"/v0/projects/:projectId/repos/:repoId/chats/ws",
		func(ctx *gin.Context) { c.agentChats.Handle(ctx) },
	)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return c, srv
}

// repoMountPredicate compiles the SAME predicate Broadcaster.Push evaluates for
// a client of .../projects/<projectID>/repos/<repoID>/chats/ws. Pushing through
// a live socket cannot prove a NEGATIVE for message_delta — that kind is
// coalesced, and drainPending's cross-key order is unspecified by contract — so
// the delivery decision is asserted where it is actually made, which is here.
func repoMountPredicate(
	t *testing.T,
	projectID string,
	repoID string,
) func(dto.AgentChatEvent) bool {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/", nil)
	ctx.Params = gin.Params{
		{Key: "projectId", Value: projectID},
		{Key: "repoId", Value: repoID},
	}
	return ws.BuildPredicate(ctx, agentChatDef())
}

// TestAgentChatScope_HomeWorkspaceResolvesItsProjectAndNoRepo pins the fact the
// whole fix rests on: a project-home chat resolves a REAL project and an empty
// repo. Empty repo is the honest answer — the home workspace owns none — and it
// is precisely why the repoId filter alone could never scope these frames.
func TestAgentChatScope_HomeWorkspaceResolvesItsProjectAndNoRepo(t *testing.T) {
	a := newAppForSnapshot(t)
	seedHomeWorkspace(t, a, "home-p2", "p2")
	c := New(a, nil)

	scope := c.agentChatScope("chat-home-p2", "home-p2")

	assert.Equal(t, "p2", scope.ProjectID, "a home chat belongs to its project")
	assert.Empty(t, scope.RepoID, "a home chat belongs to no repo")
}

// TestRegression_RepoScopedFeedNeverCarriesAnotherProjectsRepolessFrame is the
// regression guard for the cross-project data-isolation leak.
//
// matchScopeOrUnscoped forwards a frame carrying NO repoId to EVERY repo-scoped
// subscriber, so that folder rows and root bubbles — which are legitimately
// repo-less — keep repainting. The PROJECT HOME workspace also owns no repo, so
// every frame a home chat emits was repo-less too: streamed assistant text
// (message_delta) from a chat in project B was delivered, verbatim, to a socket
// scoped to a repo of project A.
//
// Every kind this feed carries is asserted, not just the one a UI symptom was
// traced to: the escape hatch is in the FILTER, so it applied to all of them.
func TestRegression_RepoScopedFeedNeverCarriesAnotherProjectsRepolessFrame(t *testing.T) {
	inProjectA := repoMountPredicate(t, "p1", "r1")

	kinds := []struct {
		name  string
		frame dto.AgentChatEvent
	}{
		{
			name: "streamed assistant text",
			frame: dto.AgentChatEvent{
				ChatID:    "chat-home-p2",
				ProjectID: "p2",
				Kind:      dto.AgentChatKindMessageDelta,
				Message:   &dto.AgentStreamingMessageDTO{ID: "m1", Text: "project B's private reply"},
			},
		},
		{
			name: "the agent's plan for the turn",
			frame: dto.AgentChatEvent{
				ChatID:    "chat-home-p2",
				ProjectID: "p2",
				Kind:      dto.AgentChatKindPlan,
				Plan:      []dto.AgentPlanStepDTO{{Text: "project B's private plan"}},
			},
		},
		{
			name:  "a lifecycle edge",
			frame: dto.AgentChatEvent{ChatID: "chat-home-p2", ProjectID: "p2", Kind: "turn_started"},
		},
		{
			name:  "a compaction edge",
			frame: dto.AgentChatEvent{ChatID: "chat-home-p2", ProjectID: "p2", Kind: dto.AgentChatKindCompactionStarted},
		},
		{
			name: "a folder row",
			frame: dto.AgentChatEvent{
				FolderID:  "f-in-p2",
				ProjectID: "p2",
				Kind:      "folder_created",
			},
		},
	}

	for _, k := range kinds {
		t.Run(k.name, func(t *testing.T) {
			assert.False(t, inProjectA(k.frame),
				"a repo-scoped subscriber in project A must never receive project B's repo-less frames")
		})
	}
}

// TestRegression_RepoScopedFeedStillCarriesItsOwnProjectsRepolessFrame is the
// other half, and the behaviour the fix must NOT break.
//
// "No repo" is a legitimate answer for a folder row, a root bubble, and every
// project-home chat, and those rows are drawn in the Chats panel of every repo
// of their own project. Holding a repo-less frame to an exact repo would drop
// them from every repo-scoped subscriber — the silent-folder-feed death
// matchScopeOrUnscoped exists to avoid. The frame is held to the PROJECT
// instead, which is the narrowest scope that still keeps them.
func TestRegression_RepoScopedFeedStillCarriesItsOwnProjectsRepolessFrame(t *testing.T) {
	inProjectA := repoMountPredicate(t, "p1", "r1")

	assert.True(t, inProjectA(dto.AgentChatEvent{
		ChatID:    "chat-home-p1",
		ProjectID: "p1",
		Kind:      dto.AgentChatKindMessageDelta,
		Message:   &dto.AgentStreamingMessageDTO{ID: "m1", Text: "same project"},
	}), "a repo-less frame from this subscriber's OWN project still reaches it")

	assert.True(t, inProjectA(dto.AgentChatEvent{
		FolderID:  "f-in-p1",
		ProjectID: "p1",
		Kind:      "folder_created",
	}), "the live folder feed of this subscriber's own project survives")

	assert.True(t, inProjectA(dto.AgentChatEvent{
		ChatID: "orphan",
		Kind:   "turn_started",
	}), "a row that resolves NEITHER project nor repo still reaches everyone, unchanged")

	assert.False(t, inProjectA(dto.AgentChatEvent{
		ChatID:    "chat-in-r2",
		ProjectID: "p1",
		RepoID:    "r2",
		Kind:      "turn_started",
	}), "a frame that knows its repo is still held to exactly that repo")
}

// TestRegression_RepoScopedChatWSNeverCarriesAnotherProjectsHomeChat proves the
// same leak over a LIVE socket, end to end from the hub.Subscriber push site.
//
// Project B's frame is pushed FIRST and B's own connection is required to
// observe it: that is a positive proof the frame fired and cleared the WS
// layer, so "the frame A received is A's" is a proof of isolation rather than a
// proof that A eventually gets its own. It is also the same-project half of the
// invariant — B's subscriber is scoped to a REPO, and the home chat it receives
// has none.
//
// Both pushes use an ordered (non-coalesced) kind so the wire order is the
// broadcaster's FIFO contract rather than drainPending's unspecified one.
func TestRegression_RepoScopedChatWSNeverCarriesAnotherProjectsHomeChat(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "A", "p1", "r1", "", "")
	seedHomeWorkspace(t, a, "home-p2", "p2")
	c, srv := serveChats(t, a)

	connA := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/chats/ws")
	connB := dialWSAt(t, srv, "/v0/projects/p2/repos/r2/chats/ws")
	c.agentChats.WaitNRegistered(2)

	c.PushAgentChat("chat-home-p2", "home-p2", "turn_started", true)

	gotB := readJSON(t, connB)
	require.Equal(t, "chat-home-p2", gotB["chatId"],
		"a repo-scoped subscriber still receives its OWN project's home-chat frames")
	require.Equal(t, "p2", gotB["projectId"], "the frame names the project it belongs to")

	c.PushAgentChat("chat-1", "A", "turn_started", true)

	gotA := readJSON(t, connA)
	assert.Equal(t, "chat-1", gotA["chatId"],
		"a repo-scoped subscriber in project A must never receive project B's home chat")
	assert.Equal(t, "p1", gotA["projectId"])
	assert.Equal(t, "r1", gotA["repoId"])
}

// TestRegression_PushAgentChatPlanIsScopedLikeEveryOtherFrame closes the second
// half of the leak: PushAgentChatPlan never set RepoID at all, so the agent's
// own running to-do list — free text the model wrote — fanned out to every
// repo-scoped subscriber on the daemon regardless of the chat's real scope.
func TestRegression_PushAgentChatPlanIsScopedLikeEveryOtherFrame(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "A", "p1", "r1", "", "")
	seedWorkspace(t, a, "B", "p2", "r2", "", "")
	c, srv := serveChats(t, a)

	connA := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/chats/ws")
	connB := dialWSAt(t, srv, "/v0/projects/p2/repos/r2/chats/ws")
	c.agentChats.WaitNRegistered(2)

	c.PushAgentChatPlan("chat-in-b", "B", []agents.PlanStep{{Text: "project B's private plan"}})

	gotB := readJSON(t, connB)
	require.Equal(t, "chat-in-b", gotB["chatId"], "B's own plan reaches B")

	c.PushAgentChatPlan("chat-1", "A", []agents.PlanStep{{Text: "project A's plan", Status: "in_progress"}})

	gotA := readJSON(t, connA)
	assert.Equal(t, "chat-1", gotA["chatId"],
		"a plan frame must be held to its own repo, exactly as every other chat frame is")
	assert.Equal(t, "p1", gotA["projectId"])
	assert.Equal(t, "r1", gotA["repoId"])
	assert.Equal(t, dto.AgentChatKindPlan, gotA["kind"])
}

// TestRegression_PushAgentChatCompactionIsScopedLikeEveryOtherFrame is the same
// omission in the compaction edge: it set no RepoID either, so a compact_pre in
// one repo announced itself to every other repo's subscribers.
func TestRegression_PushAgentChatCompactionIsScopedLikeEveryOtherFrame(t *testing.T) {
	a := newAppForSnapshot(t)
	seedWorkspace(t, a, "A", "p1", "r1", "", "")
	seedWorkspace(t, a, "B", "p2", "r2", "", "")
	c, srv := serveChats(t, a)

	connA := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/chats/ws")
	connB := dialWSAt(t, srv, "/v0/projects/p2/repos/r2/chats/ws")
	c.agentChats.WaitNRegistered(2)

	c.PushAgentChatCompaction("chat-in-b", "B", true)

	gotB := readJSON(t, connB)
	require.Equal(t, "chat-in-b", gotB["chatId"], "B's own compaction edge reaches B")

	c.PushAgentChatCompaction("chat-1", "A", true)

	gotA := readJSON(t, connA)
	assert.Equal(t, "chat-1", gotA["chatId"],
		"a compaction frame must be held to its own repo, exactly as every other chat frame is")
	assert.Equal(t, "p1", gotA["projectId"])
	assert.Equal(t, "r1", gotA["repoId"])
	assert.Equal(t, dto.AgentChatKindCompactionStarted, gotA["kind"])
}

// TestPushChatWorktree_CarriesBothScopingAnswers proves the worktree_state frame
// names its project as well as its repo, straight off the workspace it
// describes — no forest walk, the same way RepoID already came.
func TestPushChatWorktree_CarriesBothScopingAnswers(t *testing.T) {
	a := newAppForSnapshot(t)
	c, srv := serveChats(t, a)

	conn := dialWSAt(t, srv, "/v0/projects/p1/repos/r1/chats/ws")
	c.agentChats.WaitRegistered()

	c.PushWorkspace(dto.WorkspaceDTO{ID: "w1", ProjectID: "p1", RepoID: "r1", OwningChatID: "chat-1"})

	got := readJSON(t, conn)
	assert.Equal(t, dto.AgentChatKindWorktreeState, got["kind"])
	assert.Equal(t, "p1", got["projectId"])
	assert.Equal(t, "r1", got["repoId"])
}
