//go:build integration

package tests

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Resuming a dormant chat failed with "agentrunner not found", and it was common
// for providers that bind via their own connection identity: they announce no
// conversation, so the chat had no conversation row; it had never been switched,
// so it had no provider-switch marker; and if it was minted before the chat
// aggregate carried its own vendor, it had no stored provider either. Every
// source the resolver consulted was empty, and the refusal it produced named a
// missing RUNNER — which is the ordinary dormant state and the entire reason
// Resume was being called.
//
// The daemon knew the answer the whole time. It records a PLACEMENT whenever it
// points a runner at a chat: on the reporter's own store, every runner.started
// event going back days carried both its provider and its chat, covering every
// chat on the machine. Nothing read it.
//
// These tests drive the REAL endpoints. Every one of them also asserts the chat
// COUNT is unchanged across the resume, because the failure mode being closed
// here is a resolution, never a create: a resume that invents a chat is the
// phantom-chat bug, not a fix for this one.

// chatCount counts the conversations a repo scope lists — the guard every test
// below takes before and after a resume.
func chatCount(
	t *testing.T,
	h *harness,
	imported importedRepo,
) int {
	t.Helper()
	var rows []agentChatDTO
	h.get(repoBase(imported)+"/chats", &rows)
	return len(conversationsOnly(rows))
}

// seedLegacyDormantChat builds the exact shape the reporter's store is full of: a
// chat with NO conversation row, NO interruption ledger entry and NO stored
// provider, whose ONLY trace of ever having run is the runner history.
//
// It is built through the real aggregates rather than by hand-writing rows. The
// chat is minted with no provider (what every chat minted before the vendor field
// existed carries), and a runner is then started on it and exited — announcing no
// session, exactly as a provider that binds by its own connection identity never
// does. The exit deletes the live row, which is what makes the chat dormant.
func seedLegacyDormantChat(
	t *testing.T,
	h *harness,
	imported importedRepo,
	providerID string,
) (chatID string, runnerID string) {
	t.Helper()
	ctx := t.Context()

	chatID, err := h.app.Usecases.AgentChat.MintChat(ctx, imported.workspaceID, "", "")
	require.NoError(t, err)

	runnerID = uuid.NewString()
	_, err = h.app.Repositories.AgentRunner.Start(ctx, agentrunner.StartInput{
		RunnerID:    runnerID,
		WorkspaceID: imported.workspaceID,
		ProviderID:  providerID,
		ChatID:      chatID,
		Now:         time.Now(),
	})
	require.NoError(t, err)
	_, err = h.app.Repositories.AgentRunner.Exit(ctx, runnerID, time.Now())
	require.NoError(t, err)
	h.Quiesce()

	return chatID, runnerID
}

// bootBackfill runs what the upgrading boot does to a legacy row: the boot
// reconcile, then the one-off restate of each chat's own vendor from its
// runner history.
func bootBackfill(t *testing.T, h *harness) {
	t.Helper()
	require.NoError(t, h.app.Usecases.AgentRunner.ReconcileRunnersOnBoot(t.Context()))
	require.True(t, h.app.Usecases.AgentRunner.RestateProvidersFromHistory(t.Context()))
	h.Quiesce()
}

// TestRegression_ResumeDormantChatRecoversItsProviderFromRunnerHistory is the
// reported bug, end to end. The chat has nothing the resolver used to read, and
// resuming it answered "agentrunner not found"; it must instead come back on the
// vendor the runner history names.
func TestRegression_ResumeDormantChatRecoversItsProviderFromRunnerHistory(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, _ := seedLegacyDormantChat(t, h, imported, "quietstub")
	bootBackfill(t, h)
	before := chatCount(t, h, imported)

	// Every precondition of the bug, asserted rather than assumed: take any one of
	// them away and this test proves something easier than the reported failure.
	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID, "the chat must be dormant")
	require.Empty(t, detail.sessionIDs(), "no conversation was ever announced on it")
	require.Empty(t, readInterruptions(t, h, imported, chatID), "and it was never switched")
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"placement history is the only thing left that knows, and the wire must say so")

	var revived struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats/"+chatID+"/resume", nil, http.StatusOK, &revived)
	h.QuiesceReactors()
	require.NotEmpty(t, revived.ID)

	back := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Equal(t, "quietstub", back.ActiveProviderID,
		"the chat comes back as the vendor that ran in it; guessing one converts it")
	assert.Empty(t, readInterruptions(t, h, imported, chatID),
		"and a revive is not a switch: it must draw no provider-changed divider")
	assert.Equal(t, before, chatCount(t, h, imported),
		"a resume RESOLVES a provider for a chat that exists; it must never create one")
}

// TestRegression_ResumeFromRunnerHistoryPicksTheNEWESTPlacement holds the
// recovered answer to the same ordering rule the other sources obey. A chat
// handed from one vendor to another and then stopped must come back as the one
// that was actually running — reading placement history as an unordered set would
// resurrect whichever row happened to be first, which is the stale-provider bug
// the conversation ordering was already fixed for once.
func TestRegression_ResumeFromRunnerHistoryPicksTheNEWESTPlacement(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	ctx := t.Context()
	chatID, firstRunner := seedLegacyDormantChat(t, h, imported, "streamstub")

	second := uuid.NewString()
	_, err := h.app.Repositories.AgentRunner.Start(ctx, agentrunner.StartInput{
		RunnerID:    second,
		WorkspaceID: imported.workspaceID,
		ProviderID:  "quietstub",
		ChatID:      chatID,
		Now:         time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	_, err = h.app.Repositories.AgentRunner.Exit(ctx, second, time.Now().Add(time.Minute))
	require.NoError(t, err)
	h.Quiesce()
	require.NotEqual(t, firstRunner, second)
	bootBackfill(t, h)

	before := chatCount(t, h, imported)

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID)
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"the LAST vendor placed on the chat is the one that was running")

	h.post(repoBase(imported)+"/chats/"+chatID+"/resume", nil, http.StatusOK, nil)
	h.QuiesceReactors()

	back := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Equal(t, "quietstub", back.ActiveProviderID)
	assert.Equal(t, before, chatCount(t, h, imported))
}

// TestRegression_FreshlyCreatedChatCarriesItsProviderFromBirth is the other half
// of the fix: a chat created from now on records its vendor when the ROW is
// minted, not only if and when a CLI successfully spawns on it. The spawn-time
// write is best-effort and runs after a fork that may never happen, so it could
// leave a chat that could never say what it ran.
//
// It is proved where nothing else can answer instead: the runner is stopped AND
// its placement history forgotten, so the chat's own stored provider is the only
// surviving source. That is the field Part A writes at birth.
func TestRegression_FreshlyCreatedChatCarriesItsProviderFromBirth(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, _ := createStubChat(t, h, imported, "quietstub")
	before := chatCount(t, h, imported)

	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	stored, err := h.app.Usecases.AgentChat.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "quietstub", stored.ProviderID,
		"the chat aggregate records its vendor from the mint, not from a later spawn")

	// Take away every OTHER source, so only the durable field can answer.
	require.NoError(t, h.app.Repositories.AgentRunner.ForgetChat(t.Context(), chatID))
	h.Quiesce()

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID)
	require.Empty(t, detail.sessionIDs())
	require.Empty(t, readInterruptions(t, h, imported, chatID))
	assert.Equal(t, "quietstub", detail.ActiveProviderID)

	h.post(repoBase(imported)+"/chats/"+chatID+"/resume", nil, http.StatusOK, nil)
	h.QuiesceReactors()

	back := getAgentChat(t, h, repoBase(imported), chatID)
	assert.Equal(t, "quietstub", back.ActiveProviderID)
	assert.Equal(t, before, chatCount(t, h, imported))
}

// TestRegression_ResumeWithNoEvidenceAnywhereFailsHonestlyAndCreatesNoChat is the
// floor under the other three. A chat no CLI has ever been placed on genuinely
// cannot be resolved, and that answer must stay reachable — a resolver that
// guessed instead is what silently converted a dormant chat to another vendor.
//
// Two things are asserted about the refusal. It is HONEST: the message names the
// provider as the thing that could not be resolved, never a missing runner, which
// is expected here and was never what failed. And it is still a 404, not a 500 —
// the request is well-formed and the chat exists; what is missing is the record
// being resolved.
func TestRegression_ResumeWithNoEvidenceAnywhereFailsHonestlyAndCreatesNoChat(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, err := h.app.Usecases.AgentChat.MintChat(
		t.Context(), imported.workspaceID, "", "")
	require.NoError(t, err)
	h.Quiesce()

	before := chatCount(t, h, imported)

	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID)
	require.Empty(t, detail.sessionIDs())
	require.Empty(t, readInterruptions(t, h, imported, chatID))
	require.Empty(t, detail.ActiveProviderID, "nothing has ever run here, and that is the truth")

	msg := h.mutationError(http.MethodPost,
		repoBase(imported)+"/chats/"+chatID+"/resume", nil, http.StatusNotFound)
	assert.Contains(t, msg, "no longer records which provider it ran")
	assert.NotContains(t, msg, "agentrunner: not found",
		"a dormant chat has no runner BY DEFINITION; saying so named the wrong missing thing")

	assert.Equal(t, before, chatCount(t, h, imported),
		"a refused resume must leave the forest exactly as it was")
}
