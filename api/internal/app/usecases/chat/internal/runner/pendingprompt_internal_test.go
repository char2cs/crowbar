package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// pendingPromptFixture is settleFixture's setup without the MarkSpawned step,
// so a test can drive the journal record through whichever real
// agentjournal transition it needs (including ones settleFixture never
// reaches, like MarkFailedDispatch) and can set real prompt text.
func pendingPromptFixture(t *testing.T) (*Runners, string) {
	t.Helper()
	chatsDir := filepath.Join(t.TempDir(), "chats")

	rs := &Runners{
		ws:            fakeWSReader{chatsDir: chatsDir},
		chats:         stubChatsForSettle{chat: domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}},
		conversations: stubConversationsForSettle{},
		prompts:       agentjournal.NewPromptRequests(),
	}
	return rs, rs.prompts.Dir(chatsDir, "chat-1")
}

// TestRegression_PendingPrompt_RecoversSpawnedRequestID pins the happy path a
// prior version of PendingPrompt got wrong twice: a spawned record IS the
// case this method exists to recover, and RequestID must survive the trip so
// the client can reconcile the recovered row against its own queue entry.
func TestRegression_PendingPrompt_RecoversSpawnedRequestID(t *testing.T) {
	rs, dir := pendingPromptFixture(t)
	now := time.Now()

	_, existing, err := rs.prompts.Begin(
		dir, "req-1", "hello from a lost tab", "hash-1", "codex", "runner-1", "runner-1", now,
	)
	require.NoError(t, err)
	require.False(t, existing)
	_, err = rs.prompts.MarkSpawned(dir, "req-1", "hash-1", "runner-1", "term-1", now)
	require.NoError(t, err)

	pending, found, err := rs.PendingPrompt(context.Background(), "chat-1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "req-1", pending.RequestID)
	require.Equal(t, "hello from a lost tab", pending.Text)
	require.Equal(t, agentjournal.PromptStateSpawned, pending.State)
}

// TestRegression_PendingPrompt_RefusesSettledRecord pins the actual bug: a
// provider built-in like /compact is journalled through this same prompt path,
// and settles with no ledger turn ever produced to confirm it. Recovering a
// settled record would hand the client a ghost queue row it can never
// dispatch or resolve, wedging the chat's prompt FIFO head forever.
func TestRegression_PendingPrompt_RefusesSettledRecord(t *testing.T) {
	rs, _ := settleFixture(t)

	retired, err := rs.SettleDelivery(context.Background(), "chat-1", "req-1")
	require.NoError(t, err)
	require.True(t, retired)

	_, found, err := rs.PendingPrompt(context.Background(), "chat-1")
	require.NoError(t, err)
	require.False(t, found)
}

// TestRegression_PendingPrompt_RefusesFailedDispatchRecord pins the other
// terminal state the allowlist excludes: a proven pre-spawn failure is just as
// final as settled and must not come back as something still worth recovering.
func TestRegression_PendingPrompt_RefusesFailedDispatchRecord(t *testing.T) {
	rs, dir := pendingPromptFixture(t)
	now := time.Now()

	_, existing, err := rs.prompts.Begin(
		dir, "req-2", "another lost prompt", "hash-2", "codex", "runner-1", "runner-1", now,
	)
	require.NoError(t, err)
	require.False(t, existing)
	require.NoError(t, rs.prompts.MarkFailedDispatch(dir, "req-2", now))

	_, found, err := rs.PendingPrompt(context.Background(), "chat-1")
	require.NoError(t, err)
	require.False(t, found)
}
