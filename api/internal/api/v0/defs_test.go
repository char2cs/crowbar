package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

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

func TestAgentChatDef_Lambdas(t *testing.T) {
	def := agentChatDef()
	evt := dto.AgentChatEvent{ChatID: "c1", WorkspaceID: "w1", ProjectID: "p1", RepoID: "r1", Kind: "bound"}

	assert.Equal(t, "w1", def.Namespace(evt))
	assert.True(t, def.FlatNamespace)

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "c1")
	assert.Contains(t, string(data), "bound")

	// FOUR filters, each inactive where its param is unbound: wsId narrows the
	// HOME mount (RequireHomeWorkspace injects a :wsId for it to resolve),
	// projectId and repoId together narrow the REPO mount, which binds no :wsId
	// at all and was therefore scoped by nothing before repoId existed — and
	// still leaked every REPO-LESS frame across projects before projectId did —
	// and chatId narrows the per-CHAT mount (/v0/chats/:chatId/ws) that replaces
	// watching one workspace's stream, and that carries the provider poll with it.
	require.Len(t, def.Filters, 4)
	assert.Equal(t, "wsId", def.Filters[0].Param)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
	assert.Equal(t, "projectId", def.Filters[1].Param)
	assert.Equal(t, "p1", def.Filters[1].Extract(evt))
	assert.Equal(t, "repoId", def.Filters[2].Param)
	assert.Equal(t, "r1", def.Filters[2].Extract(evt))
	assert.Equal(t, "chatId", def.Filters[3].Param)
	assert.Equal(t, "c1", def.Filters[3].Extract(evt))

	// No snapshot: a freshly-connected client waits for the next lifecycle
	// event rather than replaying a "current state".
	assert.Nil(t, def.Snapshot(""))
}

// TestMatchScopeOrUnscoped_HoldsAKnownIDAndLetsAnUnknownOneThrough pins the
// asymmetry the projectId and repoId filters both turn on, and the reason
// neither is ws.ExactMatch.
//
// A frame that KNOWS a scoping id is held to exactly that id — that is the
// whole fix. A frame that CANNOT know it reaches everyone, because half the
// rows on this feed have no such id to be held to: a FOLDER carries neither a
// workspace nor a repo id, so does a bubble whose ancestry owns no workspace,
// and so does every row in a project home, whose workspace owns no repo.
// ExactMatch would drop those frames from every repo-scoped subscriber, which
// silently kills the live folder feed the Chats panel repaints from.
//
// The hatch opening per FIELD is what made one filter insufficient: it is why a
// repo-less frame escaped scoping altogether until the projectId filter — using
// this same matcher on a field those rows CAN answer — bounded it.
func TestMatchScopeOrUnscoped_HoldsAKnownIDAndLetsAnUnknownOneThrough(t *testing.T) {
	assert.True(t, matchScopeOrUnscoped("r1", "r1"), "a frame from this repo is delivered")
	assert.False(t, matchScopeOrUnscoped("r1", "r2"), "a frame from another repo is not")
	assert.True(t, matchScopeOrUnscoped("r1", ""), "a frame with no repo to be held to reaches everyone")
	assert.True(t, matchScopeOrUnscoped("p1", "p1"), "a frame from this project is delivered")
	assert.False(t, matchScopeOrUnscoped("p1", "p2"), "a frame from another project is not")
}
