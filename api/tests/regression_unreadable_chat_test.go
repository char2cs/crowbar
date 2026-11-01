//go:build integration

package tests

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// One chat whose stored ledger row can no longer be read breaks that chat
// alone: its repo's chat list still answers with every chat, a sibling still
// opens, and the broken chat's feed is refused as unreadable.
func TestRegression_AnUnreadableLedgerRowBreaksOnlyItsChat(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeProviderDescriptor(t, h1, "streamstub", streamStubProviderDescriptorYAML)
	imported := importWritableWorkspace(t, h1)
	healthy, healthyRunner := createStubChat(t, h1, imported, "streamstub")
	broken, brokenRunner := createStubChat(t, h1, imported, "streamstub")
	postHook(t, h1, repoBase(imported), "streamstub", healthyRunner, "session_start", `{"session_id":"sess-ok"}`)
	postHook(t, h1, repoBase(imported), "streamstub", brokenRunner, "session_start", `{"session_id":"sess-bad"}`)
	h1.Quiesce()
	h1.shutdown()

	db := openSQLite(t, filepath.Join(homeAt(t, home).Store(), ledgerstore.FileName))
	corrupted := db.Exec(`UPDATE chat_ledger SET at = 'not-a-time' WHERE chat_id = ? AND seq = 1`, broken)
	require.NoError(t, corrupted.Error)
	require.Equal(t, int64(1), corrupted.RowsAffected, "precondition: the broken chat has a first row")
	closeSQLite(t, db)

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	listed := map[string]bool{}
	for _, row := range listChats(t, h2, imported.projectID, imported.repoID) {
		listed[row.ID] = true
	}
	assert.True(t, listed[healthy], "the healthy chat is listed")
	assert.True(t, listed[broken], "the unreadable chat is still listed")
	assert.True(t, listed[imported.chatID], "the worktree's own chat is listed")

	assert.Equal(t, []string{"sess-ok"}, readChatSession(t, h2, imported, healthy).sessionIDs())
	seqs, _ := untilReady(t, dialFeed(t, h2, healthy, 0))
	assert.NotEmpty(t, seqs, "the healthy chat's history replays")

	refused := nextFeedFrame(t, dialFeed(t, h2, broken, 0))
	assert.Equal(t, stream.ReasonRefused, refused.Reason)
	require.NotNil(t, refused.Problem)
	assert.Equal(t, problem.Unavailable, refused.Problem.Code)
}
