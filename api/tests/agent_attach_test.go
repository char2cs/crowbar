//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitForAttached reads the agent-chat feed until chatID reports want attached
// clients.
func waitForAttached(
	t *testing.T,
	frames <-chan map[string]any,
	chatID string,
	want int,
) {
	t.Helper()
	for {
		f := waitForChatFrame(t, frames, chatID, "attached")
		if f["attached"] == float64(want) {
			return
		}
	}
}

func attachPath(
	chatID string,
) string {
	return "/v0/chats/" + chatID + "/attach"
}

// TestRegression_DetachingOneClientKeepsTheAgentRunningForTheOther: a chat can be
// open on several clients at once, and one of them going away (a closed pane, a
// dropped phone connection) only detaches it. The agent keeps running and every
// other client keeps receiving the chat's events.
func TestRegression_DetachingOneClientKeepsTheAgentRunningForTheOther(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, runnerID := createLiveStubChat(t, h, imported)
	feed := dialAgentWS(t, h, repoBase(imported)+"/chats/ws")

	desktop := h.dial(attachPath(chatID))
	_ = h.dial(attachPath(chatID))
	waitForAttached(t, feed, chatID, 2)

	require.NoError(t, desktop.Close())
	waitForAttached(t, feed, chatID, 1)

	h.Quiesce()
	assert.NotEmpty(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID,
		"a client detaching must not end the agent another client is still attached to")
	postAgentHook(t, h, imported, runnerID, "user_prompt", `{"prompt":"still here"}`)
	waitForChatFrame(t, feed, chatID, "turn_started")
}

// TestAgentAttach_FeedSnapshotReportsExistingViewers: a client that connects after
// others attached learns each chat's viewer count without any frame being replayed.
func TestAgentAttach_FeedSnapshotReportsExistingViewers(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createLiveStubChat(t, h, imported)
	_ = h.dial(attachPath(chatID))

	late := dialAgentWS(t, h, repoBase(imported)+"/chats/ws")

	waitForAttached(t, late, chatID, 1)
}

// TestRegression_KillEndsTheAgentWhileClientsAreStillAttached: any attached client
// may force-kill; the viewers stay attached to the now dormant chat.
func TestRegression_KillEndsTheAgentWhileClientsAreStillAttached(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	chatID, _ := createLiveStubChat(t, h, imported)
	feed := dialAgentWS(t, h, repoBase(imported)+"/chats/ws")
	_ = h.dial(attachPath(chatID))
	waitForAttached(t, feed, chatID, 1)

	_ = h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/kill", nil, http.StatusAccepted).Body.Close()
	h.Quiesce()

	assert.Empty(t, getAgentChat(t, h, repoBase(imported), chatID).LiveRunnerID)
	// The feed says so too: clients learn a chat stopped running from a snapshot
	// frame whose liveRunnerId is empty, never by asking.
	for {
		f := waitForChatFrame(t, feed, chatID, "exited")
		if snap, ok := f["chat"].(map[string]any); ok && snap["liveRunnerId"] == "" {
			break
		}
	}
}

// TestAgentAttach_ProjectHomeChatCountsToo: a project-home chat has no repo, and
// is attached to by the same flat route.
func TestAgentAttach_ProjectHomeChatCountsToo(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	home := "/v0/projects/" + imported.projectID + "/home"
	var chat struct {
		ID string `json:"id"`
	}
	h.post(home+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &chat)
	feed := dialAgentWS(t, h, home+"/chats/ws")

	_ = h.dial(attachPath(chat.ID))

	waitForAttached(t, feed, chat.ID, 1)
}
