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

type legacyPlacement struct {
	chatID, providerID string
	at                 time.Time
}

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

func providerlessChat(t *testing.T, h *harness, imported importedRepo) string {
	t.Helper()
	chatID, err := h.app.Usecases.AgentChat.MintChat(t.Context(), "", imported.workspaceID, "", "")
	require.NoError(t, err)
	return chatID
}

func TestRegression_ResumeDormantChatRecoversItsProviderFromRunnerHistory(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID := providerlessChat(t, h, imported)

	h = upgradeWithRunnerHistory(t, h, home, legacyPlacement{chatID, "quietstub", time.Now()})
	before := chatCount(t, h, imported)

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

func TestRegression_ResumeFromRunnerHistoryPicksTheNEWESTPlacement(t *testing.T) {
	home := t.TempDir()
	h := newHarnessAt(t, home)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	writeProviderDescriptor(t, h, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)
	chatID := providerlessChat(t, h, imported)

	placedAt := time.Now()
	h = upgradeWithRunnerHistory(t, h, home,
		legacyPlacement{chatID, "quietstub", placedAt.Add(time.Minute)},
		legacyPlacement{chatID, "streamstub", placedAt})
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

func TestRegression_FreshlyCreatedChatCarriesItsProviderFromBirth(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, _ := createStubChat(t, h, imported, "quietstub")
	before := chatCount(t, h, imported)

	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats/"+chatID+"/stop", nil, http.StatusNoContent)
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

func TestRegression_ResumeWithNoEvidenceAnywhereFailsHonestlyAndCreatesNoChat(t *testing.T) {
	h := newHarness(t)
	writeProviderDescriptor(t, h, "quietstub", quietStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h)

	chatID, err := h.app.Usecases.AgentChat.MintChat(
		t.Context(), "", imported.workspaceID, "", "")
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
