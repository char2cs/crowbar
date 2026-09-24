//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentChatActiveProviderID proves both GET .../chats (list)
// and GET .../chats/:id (detail) carry activeProviderId derived from the
// active segment, so the FE row glyph resolves with no extra fetch.
//
// It spawns LIVESTUB (`cat`), not stub (`true`), and that is load-bearing.
// activeProviderId is derived from the LIVE runner's provider; a `true` runner exits in
// microseconds and its exit projection deletes the runner row, so between the spawn and
// the GET the field would flip to "" and the test flakes (~1 run in 30). `cat` holds its
// PTY open, so the runner row survives the read and the assertion is deterministic.
func TestRegression_AgentChatActiveProviderID(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	// Join the reactors so the spawned runner has actually landed before the read (a plain
	// projection drain returns the moment the placement goroutine is spawned, before the
	// runner row exists — exactly when activeProviderId reads "").
	h.QuiesceReactors()
	chatID := created.ID

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, chatID, list[0].ID)
	assert.Equal(t, "livestub", list[0].ActiveProviderID)

	var detail struct {
		ActiveProviderID string `json:"activeProviderId"`
	}
	h.get(repoBase(imported)+"/chats/"+chatID, &detail)
	assert.Equal(t, "livestub", detail.ActiveProviderID)
}

// TestRegression_AgentChatActiveProviderID_ProviderThatNeverBoundAConversation is
// the showstopper this fix exists for, end to end: a chat switched to a provider
// that binds via its own connection identity (it never fires a session_start hook)
// and then goes dormant has NO conversation row for that provider at all — only an
// OLDER row from whatever ran before it. Before the fix, activeProviderId read only
// that older conversation and reported the WRONG, stale vendor (the user's own
// live report: "a chat genuinely running codex reported claude"). This drives the
// real POST .../switch and POST .../stop endpoints — never the resolver directly —
// and reads activeProviderId back through both GET .../chats (list) and
// GET .../chats/:id (detail).
func TestRegression_AgentChatActiveProviderID_ProviderThatNeverBoundAConversation(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, runnerA := createStubChat(t, h, imported, "streamstub")
	postProviderHook(t, h, imported, "streamstub", runnerA, "session_start", `{"session_id":"sess-a"}`)
	h.Quiesce()

	// Switch to quietstub — records the provider_switched marker — but never fire
	// its own session_start: the exact shape a provider that binds via its own
	// connection identity leaves, with no conversation row for it at all.
	var switched struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/switch",
		map[string]string{"provider": "quietstub"}, http.StatusOK, &switched)
	h.QuiesceReactors()

	// Stop the chat: the CLI actually running (quietstub) exits, leaving the chat
	// dormant with streamstub's conversation as its only bound row.
	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Empty(t, detail.LiveRunnerID, "the chat must be dormant for this to prove anything")
	assert.Equal(t, []string{"sess-a"}, detail.sessionIDs(),
		"only streamstub ever bound a conversation — quietstub's is the switch marker alone")
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"quietstub is who was actually running when the chat went dormant; streamstub's "+
			"conversation is merely older and must not win")

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, "quietstub", list[0].ActiveProviderID)
}

// TestRegression_AgentChatBornOnAProviderThatBindsNothing_StaysThatProviderWhenDormant
// is the live-reproduced data-integrity bug, end to end through the real
// endpoints: a chat BORN on a provider that binds by its own connection
// identity (quietstub — it never fires session_start, exactly as codex never
// does) and then killed has NO conversation row and NO provider_switched
// marker. Both runner projections are empty, activeProviderId answered "", and
// the pane read that absence as "never ran" and POSTed .../switch to the first
// ENABLED provider — silently converting a dormant codex chat to claude,
// transcript and all.
//
// Two facts prove it closed: the dormant chat still NAMES its provider on the
// wire, and POST .../resume — the call a reopen actually makes — revives that
// same provider rather than refusing and leaving the client to guess one.
func TestRegression_AgentChatBornOnAProviderThatBindsNothing_StaysThatProviderWhenDormant(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, _ := createStubChat(t, h, imported, "quietstub")

	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID, "the chat must be dormant for this to prove anything")
	require.Empty(t, detail.sessionIDs(), "precondition: quietstub bound no conversation at all")
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"a dormant chat that bound nothing still knows what it runs; \"\" is what let the UI guess")

	var listed []agentChatDTO
	h.get(repoBase(imported)+"/chats", &listed)
	list := conversationsOnly(listed)
	require.Len(t, list, 1)
	assert.Equal(t, "quietstub", list[0].ActiveProviderID)

	var revived struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/resume", nil, http.StatusOK, &revived)
	h.QuiesceReactors()

	back := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Equal(t, "quietstub", back.ActiveProviderID,
		"reopening a chat revives the vendor it was on — converting it is the bug")
	assert.Empty(t, readInterruptions(t, h, imported, chatID),
		"and a revive is not a switch: it must draw no provider-changed divider")
}
