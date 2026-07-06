package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleHandoff_WrapsLedgerEntriesInPreamble(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	appendTranscript(t, f, segID, "claude", "sid-1", "first turn transcript")
	appendTranscript(t, f, segID, "claude", "sid-1", "second turn transcript")

	got, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, "=== HANDED-OFF CONTEXT (Crowbar) ===\n"))
	assert.True(t, strings.HasSuffix(got, "\n=== END ==="))
	assert.Contains(t, got, "first turn transcript")
	assert.Contains(t, got, "second turn transcript")
	// Both entries must appear, in append order.
	assert.Less(t, strings.Index(got, "first turn transcript"), strings.Index(got, "second turn transcript"))
}

func TestAssembleHandoff_EmptyLedger_ReturnsEmptyString(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	got, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAssembleHandoff_UnknownChat_ReturnsError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, err := f.usecase.AssembleHandoff(ctx, "does-not-exist")
	require.Error(t, err)
}

// appendTranscript drives a turn_stop hook through IngestHook so a ledger
// entry gets appended for the chat behind segID, via the same path
// production code uses to populate the ledger: the assistant's turn text
// comes straight from the hook payload's last_assistant_message field, not a
// vendor transcript file.
func appendTranscript(t *testing.T, f testFixture, segID, provider, sessionID, content string) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(context.Background(), segID, provider, "turn_stop",
		mustJSON(t, map[string]any{
			"session_id":             sessionID,
			"last_assistant_message": content,
		})))
}
