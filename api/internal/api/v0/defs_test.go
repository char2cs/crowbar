package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestWorkspacesDef_Lambdas(t *testing.T) {
	def := workspacesDef(nil)
	d := dto.WorkspaceDTO{ID: "w1", ProjectID: "p1", RepoID: "r1"}

	assert.Equal(t, "p1/r1/w1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "w1")

	// Prefix-based scoping replaces the projectId/repoId query Filters (spec §5).
	assert.Empty(t, def.Filters)
}

func TestProjectsDef_NamespaceID(t *testing.T) {
	def := projectsDef(nil)
	d := dto.ProjectDTO{ID: "p1"}

	assert.Equal(t, "p1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "p1")
}

func TestReposDef_NamespaceProjectRepo(t *testing.T) {
	def := reposDef(nil)
	d := dto.RepoDTO{ID: "r1", ProjectID: "p1"}

	assert.Equal(t, "p1/r1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "r1")
}

func TestThreadsDef_NamespaceProjectRepoWsID(t *testing.T) {
	def := threadsDef(nil)
	d := dto.ThreadDTO{ID: "t1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}

	assert.Equal(t, "p1/r1/w1/t1", def.Namespace(d))

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "t1")
}

func TestThreadsDef_FiltersScopeByProjectRepoWs(t *testing.T) {
	def := threadsDef(nil)
	d := dto.ThreadDTO{ID: "t1", ProjectID: "p1", RepoID: "r1", WorkspaceID: "w1"}

	require.Len(t, def.Filters, 3)
	assert.Equal(t, "projectId", def.Filters[0].Param)
	assert.Equal(t, "p1", def.Filters[0].Extract(d))
	assert.Equal(t, "repoId", def.Filters[1].Param)
	assert.Equal(t, "r1", def.Filters[1].Extract(d))
	assert.Equal(t, "wsId", def.Filters[2].Param)
	assert.Equal(t, "w1", def.Filters[2].Extract(d))
}

func TestTerminalsDef_NamespaceChat(t *testing.T) {
	def := terminalsDef(nil, nil)
	d := dto.TerminalSessionDTO{ID: "s1", ChatID: "c1"}

	// The namespace is the OWNING CHAT, NOT the session leaf: a chat-scoped
	// subscription receives every session that chat owns. It is flat — a bare
	// chat id, never a hierarchical "p/r/w" path — so the hierarchical
	// client-scope prefix must not be applied to it.
	assert.Equal(t, "c1", def.Namespace(d))
	assert.True(t, def.FlatNamespace)

	data, err := def.Serialize(d)
	require.NoError(t, err)
	assert.Contains(t, string(data), "s1")
}

func TestTerminalsDef_FiltersScopeByChat(t *testing.T) {
	def := terminalsDef(nil, nil)
	d := dto.TerminalSessionDTO{ID: "s1", ChatID: "c1"}

	// ONE filter: the dual-served route is /v0/chats/:chatId/terminals, which
	// binds no projectId/repoId/wsId at all, so chatId is what scopes a client.
	require.Len(t, def.Filters, 1)
	assert.Equal(t, "chatId", def.Filters[0].Param)
	assert.Equal(t, "c1", def.Filters[0].Extract(d))
}

func TestTerminalsDef_SnapshotNilWithoutEngine(t *testing.T) {
	def := terminalsDef(nil, nil)
	assert.Nil(t, def.Snapshot)
}

func TestGitDef_Lambdas(t *testing.T) {
	def := gitDef(nil)
	evt := gitdomain.GitStatusEvent{
		WsID:    "w1",
		ChatIDs: []string{"chat-a", "chat-b"},
		Status:  gitdomain.GitStatus{Branch: "main"},
	}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "main")
	assert.NotContains(t, string(data), "wsId")
	assert.NotContains(t, string(data), "chat-a",
		"the fan-out set is routing, not payload: a consumer is never handed a workspace's chat roster")

	// TWO filters, one per live mount: the workspace-scoped route resolves
	// wsId, the chat-scoped one resolves chatId, and each client activates only
	// the one its own request binds.
	require.Len(t, def.Filters, 2)
	assert.Equal(t, "wsId", def.Filters[0].Param)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
	assert.Equal(t, "chatId", def.Filters[1].Param)
	assert.Equal(t, []string{"chat-a", "chat-b"}, def.Filters[1].ExtractSet(evt))
}

func TestFilesDef_Lambdas(t *testing.T) {
	def := filesDef()
	evt := domain.FileChangeEvent{WsID: "w1", Path: "a.go"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "a.go")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestAgentChatDef_Lambdas(t *testing.T) {
	def := agentChatDef()
	evt := dto.AgentChatEvent{ChatID: "c1", WorkspaceID: "w1", RepoID: "r1", Kind: "bound"}

	assert.Equal(t, "w1", def.Namespace(evt))
	assert.True(t, def.FlatNamespace)

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "c1")
	assert.Contains(t, string(data), "bound")

	// TWO filters: wsId narrows the HOME mount (RequireHomeWorkspace injects a
	// :wsId for it to resolve), and repoId narrows the REPO mount, which binds
	// no :wsId at all and was therefore scoped by nothing before repoId existed.
	require.Len(t, def.Filters, 2)
	assert.Equal(t, "wsId", def.Filters[0].Param)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
	assert.Equal(t, "repoId", def.Filters[1].Param)
	assert.Equal(t, "r1", def.Filters[1].Extract(evt))

	// No snapshot: a freshly-connected client waits for the next lifecycle
	// event rather than replaying a "current state".
	assert.Nil(t, def.Snapshot(""))
}

// TestMatchRepoOrUnscoped_HoldsAKnownRepoAndLetsAnUnknownOneThrough pins the
// asymmetry the repoId filter turns on, and the reason it is not ws.ExactMatch.
//
// A frame that KNOWS its repo is held to exactly that repo — that is the whole
// fix. A frame that CANNOT know it reaches everyone, because half the rows on
// this feed have no repo to be held to: a FOLDER carries neither a workspace
// nor a repo id, and so does a bubble whose ancestry owns no workspace.
// ExactMatch would drop those frames from every repo-scoped subscriber, which
// silently kills the live folder feed the Chats panel repaints from.
func TestMatchRepoOrUnscoped_HoldsAKnownRepoAndLetsAnUnknownOneThrough(t *testing.T) {
	assert.True(t, matchRepoOrUnscoped("r1", "r1"), "a frame from this repo is delivered")
	assert.False(t, matchRepoOrUnscoped("r1", "r2"), "a frame from another repo is not")
	assert.True(t, matchRepoOrUnscoped("r1", ""), "a frame with no repo to be held to reaches everyone")
}

func TestLSPDef_Lambdas(t *testing.T) {
	def := lspDef(nil, nil)
	evt := lspdomain.DiagnosticsEvent{WsID: "w1"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "w1")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}
