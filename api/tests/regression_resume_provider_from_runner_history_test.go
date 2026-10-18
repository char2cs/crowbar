//go:build integration

package tests

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/adapter/migrations/chatledger"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
)

// Resuming a dormant chat failed with "agentrunner not found", and it was common
// for providers that bind via their own connection identity: they announce no
// conversation, so the chat had no conversation row; it had never been switched,
// so it had no provider-switch marker; and if it was minted before the chat
// aggregate carried its own vendor, it had no stored provider either. The one
// record that still named its vendor was the old daemon's runner history.
//
// Nothing writes that history any more: the upgrade boot reads it once and
// restates each chat's own provider from it. These tests seed it between a crash
// and that boot, then drive the REAL endpoints. Every one of them also asserts the
// chat COUNT is unchanged across the resume: a resume that invents a chat is the
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

// legacyPlacement is one row of the old daemon's runner history: providerID was
// placed on chatID at at.
type legacyPlacement struct {
	chatID, providerID string
	at                 time.Time
}

// upgradeWithRunnerHistory crashes h, writes placements into the legacy runner
// history as the old daemon left it, and boots an upgrading daemon over home.
func upgradeWithRunnerHistory(
	t *testing.T,
	h *harness,
	home string,
	placements ...legacyPlacement,
) *harness {
	t.Helper()
	h.Quiesce()
	h.crash()
	db, err := gorm.Open(sqlite.Open(filepath.Join(homeAt(t, home).Store(), chatledger.RunnerDB)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(chatledger.PlacementsDDL).Error)
	for _, p := range placements {
		require.NoError(t, db.Exec(
			"INSERT INTO agent_chat_placements (chat_id, provider_id, last_placed_at) VALUES (?, ?, ?)",
			p.chatID, p.providerID, p.at).Error)
	}
	raw, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, raw.Close())
	forgetMigrations(t, home)

	upgraded := newHarnessAt(t, home)
	upgraded.Quiesce()
	return upgraded
}

// providerlessChat mints a chat with no stored provider, as every chat minted
// before the vendor field existed carries.
func providerlessChat(t *testing.T, h *harness, imported importedRepo) string {
	t.Helper()
	chatID, err := h.app.Usecases.AgentChat.MintChat(t.Context(), imported.workspaceID, "", "")
	require.NoError(t, err)
	return chatID
}

// TestRegression_ResumeDormantChatRecoversItsProviderFromRunnerHistory is the
// reported bug, end to end. The chat has nothing the resolver used to read, and
// resuming it answered "agentrunner not found"; it must instead come back on the
// vendor the runner history names.
func TestRegression_ResumeDormantChatRecoversItsProviderFromRunnerHistory(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID := providerlessChat(t, h, imported)

	h = upgradeWithRunnerHistory(t, h, home, legacyPlacement{chatID, "quietstub", time.Now()})
	before := chatCount(t, h, imported)

	// Every precondition of the bug, asserted rather than assumed: take any one of
	// them away and this test proves something easier than the reported failure.
	detail := getAgentChat(t, h, repoBase(imported), chatID)
	require.Empty(t, detail.LiveRunnerID, "the chat must be dormant")
	require.Empty(t, detail.sessionIDs(), "no conversation was ever announced on it")
	require.Empty(t, readInterruptions(t, h, imported, chatID), "and it was never switched")
	assert.Equal(t, "quietstub", detail.ActiveProviderID,
		"the runner history was the only thing left that knew, and the wire must say so")

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
// that was actually running, never whichever row happened to be read first.
func TestRegression_ResumeFromRunnerHistoryPicksTheNEWESTPlacement(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID := providerlessChat(t, h, imported)

	placedAt := time.Now()
	h = upgradeWithRunnerHistory(t, h, home,
		legacyPlacement{chatID, "streamstub", placedAt},
		legacyPlacement{chatID, "quietstub", placedAt.Add(time.Minute)})
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
// It is proved where nothing else can answer: the runner is stopped and announced
// no conversation, so the chat's own stored provider is the only source. That is
// the field Part A writes at birth.
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
	assert.NotContains(t, msg, agentusecase.ErrNoLiveRunner.Error(),
		"a dormant chat has no runner BY DEFINITION; saying so named the wrong missing thing")

	assert.Equal(t, before, chatCount(t, h, imported),
		"a refused resume must leave the forest exactly as it was")
}
