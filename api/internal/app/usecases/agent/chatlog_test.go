package agent_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
)

// TestReadChatLog_RendersTheLedger guards agent.Usecase.ReadChatLog — the
// production agenttools.ChatLogReader get_chat_log calls once a chat's
// workspace has already cleared the caller's CanSee check. Unlike
// AssembleHandoff it returns the RAW conversation with no preamble/footer
// wrapper: get_chat_log hands prose straight to a model, not an injected
// spawn-time context document.
func TestReadChatLog_RendersTheLedger(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"})))

	out, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, out, "assistant (claude): done thing")
}

// An unspoken chat's ledger is empty, and that is a NORMAL state — not an
// error and not blank text a model would read as a broken tool.
func TestReadChatLog_EmptyLedgerReadsAsNoTurns(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	out, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, agenttools.NoChatTurnsText, out)
}

func TestReadChatLog_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadChatLog(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read chat log")
}
