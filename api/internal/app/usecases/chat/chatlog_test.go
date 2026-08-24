package chat_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadChatLog_RendersTheLedger guards agent.ChatUsecase.ReadChatLog — the
// production agenttools.ChatLogReader get_chat_log calls once a chat's
// workspace has already cleared the caller's CanSee check. Unlike
// AssembleHandoff it carries the RAW conversation with no preamble/footer
// wrapper: get_chat_log hands prose straight to a model, not an injected
// spawn-time context document.
//
// It reports TURNS, not finished text, because get_chat_log caps what it returns
// and states how many turns it dropped — a count that can only be taken where
// turns are still separate values. The speaker attribution still has to be the
// ledger's own ("assistant (<provider>)"), since that is the wording every chat
// log Crowbar has produced already uses.
func TestReadChatLog_RendersTheLedger(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"})))

	out, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "assistant (claude)", out[0].Speaker)
	assert.Equal(t, "done thing", out[0].Body)
}

// An unspoken chat's ledger is empty. ReadChatLog itself returns "" (not an
// error) rather than agenttools.NoChatTurnsText: turning that into explicit
// "no turns" prose is getChatLog's job (the tool layer), the single place that
// normalization lives, since get_chat_log is ReadChatLog's only caller today —
// see TestGetChatLog_EmptyLedgerIsExplicitNotAnError in the agenttools package
// for the tool-layer half of this contract.
func TestReadChatLog_EmptyLedgerReturnsEmptyNotAnError(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	out, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestReadChatLog_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadChatLog(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read chat log")
}
