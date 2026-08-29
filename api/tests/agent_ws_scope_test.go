//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dialAgentWS opens the agent-chat lifecycle WebSocket at path (h.dial, which
// registers the connection's Close as test cleanup) and returns a channel fed
// by a background reader goroutine that decodes every text frame. The
// goroutine's only exit conditions are a real signal — a read error (the
// connection closing, e.g. on test cleanup) — never a timer; it is the
// no-sleep/no-poll counterpart to harness_test.go's readUntil, structured as a
// channel so callers can select against the test context's Done() as the sole
// backstop instead of a fixed read deadline.
func dialAgentWS(
	t *testing.T,
	h *harness,
	path string,
) <-chan map[string]any {
	t.Helper()
	conn := h.dial(path)
	frames := make(chan map[string]any, 8)
	go func() {
		defer close(frames)
		for {
			mt, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if mt != websocket.TextMessage {
				continue
			}
			var got map[string]any
			if json.Unmarshal(raw, &got) != nil {
				continue
			}
			frames <- got
		}
	}()
	return frames
}

// TestAgentWS_HomeMountIsolation proves the agent-chat lifecycle WebSocket's
// HOME mount is still scoped per project after Task 17's route rescope
// (RequireHomeWorkspace's injected :wsId feeds the SAME agentChatDef wsId
// Filter this test pinned before): a subscriber of project A's home feed
// receives A's own "created" frame and never project B's, even though B's
// chat (and its own "created" frame, positively proven on B's own connection)
// was created first. Task 17 only moved this invariant's ADDRESS — it used to
// be pinned at a workspace-scoped mount that no longer exists for chats — it
// did not touch the guard itself.
func TestAgentWS_HomeMountIsolation(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	projA := importProject(t, h)
	projB := importProject(t, h)
	homeA := "/v0/projects/" + projA.projectID + "/home"
	homeB := "/v0/projects/" + projB.projectID + "/home"

	framesA := dialAgentWS(t, h, homeA+"/chats/ws")
	framesB := dialAgentWS(t, h, homeB+"/chats/ws")

	// Create B's chat FIRST and require ITS OWN connection to observe the
	// "created" frame before A's chat is ever created: this is a real,
	// positive proof the event actually fired and reached the WS layer — not
	// an assumption — so that if the home wsId filter were broken (matching
	// every project), B's already-fired frame would be the very first thing
	// connA's channel delivers below.
	var chatB struct {
		ID string `json:"id"`
	}
	h.post(homeB+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &chatB)
	require.NotEmpty(t, chatB.ID)

	select {
	case f := <-framesB:
		require.Equal(t, "created", f["kind"])
		require.Equal(t, chatB.ID, f["chatId"], "project B's own home connection must see its own chat's created frame")
	case <-t.Context().Done():
		t.Fatal("timed out waiting for project B's own created frame")
	}

	// Now create A's chat. connA's channel is read for the first time here:
	// if isolation were broken, B's frame (already pushed above, chronologically
	// first) would arrive before A's own — so asserting the received frame IS
	// A's is a real proof of isolation, not merely of "A eventually gets its
	// own frame".
	var chatA struct {
		ID string `json:"id"`
	}
	h.post(homeA+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &chatA)
	require.NotEmpty(t, chatA.ID)

	select {
	case f := <-framesA:
		assert.Equal(t, "created", f["kind"])
		assert.Equal(t, chatA.ID, f["chatId"], "project A's home subscriber must never see project B's chat")
	case <-t.Context().Done():
		t.Fatal("timed out waiting for project A's created frame")
	}
}

// TestAgentWS_RepoScopedSpansEveryWorkspace proves the repo-scoped mount's WS
// is deliberately NOT scoped per workspace after Task 17: the route carries no
// :wsId path segment, so agentChatDef's wsId Filter has no value anywhere to
// resolve and scopes nothing. What DOES scope the mount is a separate repoId
// Filter (agentChatDef stays FlatNamespace, matching every other feed in this
// file), matched with matchRepoOrUnscoped against the frame's own resolved
// RepoID — so ONE subscriber of a repo's feed sees a "created" frame for a
// chat born in EITHER of that repo's own workspaces, and
// (TestAgentWS_RepoScopedNeverCarriesAnotherReposChats) none at all from
// another repo.
//
// This replaces the isolation TestAgentWS_WorkspaceIsolation used to pin at
// this exact mount before the rescope: the model spec's own §5.1 relaxation
// ("chats are addressed by id alone now") means there is no workspace edge
// left here to keep two subscribers apart, so the honest invariant to pin is
// that they are NOT kept apart, on purpose.
func TestAgentWS_RepoScopedSpansEveryWorkspace(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	imported := importProject(t, h)
	mainWS := imported.workspaceID
	otherWS := createChildWorkspace(t, h, repoBase(imported), "feature/other", mainWS)
	h.Quiesce()

	frames := dialAgentWS(t, h, repoBase(imported)+"/chats/ws")

	var chatMain struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": mainWS},
		http.StatusCreated, &chatMain)
	require.NotEmpty(t, chatMain.ID)
	// waitForChatFrame drains past any OTHER frame kind (e.g. the create's own
	// permission_level_set) on the way to the one this asserts on — the repo
	// mount is unfiltered, so it carries every chat's traffic, not just "created".
	waitForChatFrame(t, frames, chatMain.ID, "created")

	var chatOther struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": otherWS},
		http.StatusCreated, &chatOther)
	require.NotEmpty(t, chatOther.ID)

	waitForChatFrame(t, frames, chatOther.ID, "created")
}

// TestAgentWS_RepoScopedNeverCarriesAnotherReposChats is the negative half of
// the test above, and the regression guard for the bug the two of them
// together describe: while agentChatDef was FlatNamespace, the repo mount's
// only scoping mechanism was a wsId Filter that resolves INACTIVE there, so a
// repo-scoped subscriber received every OTHER repo's chat events too.
//
// It is structured exactly like TestAgentWS_HomeMountIsolation, and for the
// same reason: repo B's chat is created FIRST and its own connection is
// required to observe the frame, so the event is proved to have fired and
// reached the WS layer. Only then is A's chat created, so if the scoping were
// broken the first frame A's channel delivers would be B's — which makes
// "the frame A received is A's" a real proof of isolation rather than a proof
// that A eventually gets its own.
func TestAgentWS_RepoScopedNeverCarriesAnotherReposChats(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)

	repoA := importProject(t, h)
	repoB := importProject(t, h)

	framesA := dialAgentWS(t, h, repoBase(repoA)+"/chats/ws")
	framesB := dialAgentWS(t, h, repoBase(repoB)+"/chats/ws")

	var chatB struct {
		ID string `json:"id"`
	}
	h.post(repoBase(repoB)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": repoB.workspaceID},
		http.StatusCreated, &chatB)
	require.NotEmpty(t, chatB.ID)
	waitForChatFrame(t, framesB, chatB.ID, "created")

	var chatA struct {
		ID string `json:"id"`
	}
	h.post(repoBase(repoA)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": repoA.workspaceID},
		http.StatusCreated, &chatA)
	require.NotEmpty(t, chatA.ID)

	select {
	case f := <-framesA:
		assert.Equal(t, chatA.ID, f["chatId"],
			"a repo-scoped subscriber must never receive another repo's chat frames")
	case <-t.Context().Done():
		t.Fatal("timed out waiting for repo A's own frame")
	}
}
