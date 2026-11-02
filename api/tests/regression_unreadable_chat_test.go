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
// opens, the projects scope snapshots both, and the broken chat's feed is
// refused as unreadable.
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
	snapshot := sidebarSnapshot(t, dialStream(t, h2))
	assert.Contains(t, snapshot, "chat/"+healthy, "the projects scope snapshots the healthy chat")
	assert.Contains(t, snapshot, "chat/"+broken, "and the unreadable one, as its list does")

	assert.Equal(t, []string{"sess-ok"}, readChatSession(t, h2, imported, healthy).sessionIDs())
	seqs, _ := untilReady(t, dialFeed(t, h2, healthy, 0))
	assert.NotEmpty(t, seqs, "the healthy chat's history replays")

	refused := nextFeedFrame(t, dialFeed(t, h2, broken, 0))
	assert.Equal(t, stream.ReasonRefused, refused.Reason)
	require.NotNil(t, refused.Problem)
	assert.Equal(t, problem.Unavailable, refused.Problem.Code)
}

// One chat whose read-model row will not decode leaves its own repo's list and
// nothing else: the repo and the project home still read, with every other row.
func TestRegression_AnUndecodableChatRowLeavesOnlyThatChat(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)
	writeProviderDescriptor(t, h1, "streamstub", streamStubProviderDescriptorYAML)
	writeLiveStubProviderDescriptor(t, h1)
	imported := importWritableWorkspace(t, h1)
	healthy, _ := createStubChat(t, h1, imported, "streamstub")
	broken, _ := createStubChat(t, h1, imported, "streamstub")
	homeChat := createHomeChat(t, h1, imported.projectID)
	h1.Quiesce()
	h1.shutdown()

	db := openSQLite(t, filepath.Join(homeAt(t, home).Store(), "agent_chat.db"))
	corrupted := db.Exec(`UPDATE agent_chats_read SET data = ? WHERE id = ?`, []byte("{not json"), broken)
	require.NoError(t, corrupted.Error)
	require.Equal(t, int64(1), corrupted.RowsAffected, "precondition: the broken chat has a read row")
	closeSQLite(t, db)

	h2 := newHarnessAt(t, home)
	h2.Quiesce()

	listed := map[string]bool{}
	for _, row := range listChats(t, h2, imported.projectID, imported.repoID) {
		listed[row.ID] = true
	}
	assert.True(t, listed[healthy], "the repo's list still answers with its healthy chat")
	assert.False(t, listed[broken], "and leaves the undecodable one out")
	c := dialStream(t, h2)
	c.Sub(t, "projects", 0)
	snapshot := map[string]bool{}
	synced := c.Next(t, func(f stream.Frame) bool {
		snapshot[f.Kind+"/"+f.ID] = f.T == stream.FrameEntity
		return isSynced("projects")(f)
	})
	assert.Empty(t, synced.Unread, "no repo or home is unread over one bad row")
	assert.True(t, snapshot["chat/"+healthy], "the healthy chat is snapshotted")
	assert.True(t, snapshot["chat/"+homeChat], "and so is the home's chat")
}
