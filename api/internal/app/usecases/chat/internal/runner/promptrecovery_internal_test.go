package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubConversationsWithTurn answers ChatTurns with exactly the rows given —
// unlike stubConversationsForSettle's deliberately empty ledger, this lets a
// test assert on a delivery the ledger genuinely DOES have a turn for.
type stubConversationsWithTurn struct {
	Conversations
	turns []domain.LedgerTurn
}

func (s stubConversationsWithTurn) ChatTurns(
	context.Context, string,
) ([]domain.LedgerTurn, error) {
	return s.turns, nil
}

// TestRegression_PromptRecordAccepted_MergedDeliveryNeverConfirms is a SECOND
// bug in the same family as the switch+send fixes already landed on this
// branch (sessionLegacyMinAge, mergeLeadingPositional, the RecordInjection
// promptMessage=="" gate) — found auditing the SAME code for more of them.
//
// promptRecordAccepted (the ONE function every "was this prompt actually
// delivered" check funnels through — ConfirmPromptAccepted's own hash check
// already can't match a merged delivery either, but THIS is the fallback
// every other caller relies on: reconcilePromptRunnerDeparture when the CLI
// exits, ReconcilePendingPromptFromLedger, and settleDelivery's own
// pre-settle reconciliation) requires the LEDGER TURN's own text to hash to
// the SAME value the journal stored at dispatch time:
//
//	agentjournal.PromptTextHash(t.Text) == record.TextHash
//
// The journal stores the hash of the user's ORIGINAL typed text (computed in
// Runners.SubmitPrompt before resolvePromptDelivery ever runs). But
// mergeLeadingPositional now folds the injected gap ahead of that text into
// ONE combined positional, and turn.go's handleUserPrompt records exactly
// what the hook delivers (ev.Message, the full combined string) as the
// ledger turn's Text. Hash of "context...\n\nmessage" can never equal hash of
// "message" alone — so a merged delivery can NEVER be recognized as accepted
// through this path, no matter how correctly it was delivered or answered.
//
// Consequence, confirmed by tracing every caller: the delivery is left
// sitting in PromptStateSpawned forever until the terminal-wait sweep's
// generic 30s-quiet SettleDelivery times it out into PromptStateSettled with
// consumed=false — the EXACT "provider never picked this prompt up" signal
// the abandoned-prompt path broadcasts to the client, which marks the user's
// own composer row failed with "retry or edit it" — even though the provider
// answered correctly and the reply is sitting right there in the transcript.
// This is likely why the ORIGINAL live bug report's queued item stayed
// visibly stuck even after the delivery mechanism itself was fixed.
func TestRegression_PromptRecordAccepted_MergedDeliveryNeverConfirms(t *testing.T) {
	chatsDir := filepath.Join(t.TempDir(), "chats")
	originalText := "what did I talk about with codex?"
	deliveredText := "<system-reminder>...gap the user was away for...</system-reminder>\n\n" + originalText

	chat := domain.Chat{ID: "chat-1", WorkspaceID: "ws-1"}
	rs := &Runners{
		ws:    fakeWSReader{chatsDir: chatsDir},
		chats: stubChatsForSettle{chat: chat},
		conversations: stubConversationsWithTurn{turns: []domain.LedgerTurn{
			// Exactly what turn.go's handleUserPrompt records: role/provider/
			// runnerID/timing all correctly attributed to THIS delivery —
			// deliveredThisRequest's own checks would happily match this row.
			// Only the merged TEXT differs from what the journal expects.
			{Role: "user", Provider: "claude", RunnerID: "runner-1", Text: deliveredText, At: time.Now()},
		}},
		prompts: agentjournal.NewPromptRequests(),
	}
	dir := rs.prompts.Dir(chatsDir, "chat-1")
	dispatchedAt := time.Now().Add(-time.Second)

	textHash := agentjournal.PromptTextHash(originalText)
	_, existing, err := rs.prompts.Begin(
		dir, "req-1", originalText, textHash, "claude", "runner-0", "runner-1", dispatchedAt,
	)
	require.NoError(t, err)
	require.False(t, existing)
	record, err := rs.prompts.MarkSpawned(dir, "req-1", textHash, "runner-1", "term-1", dispatchedAt)
	require.NoError(t, err)

	accepted, err := rs.promptRecordAccepted(context.Background(), chat, record)

	require.NoError(t, err)
	assert.True(t, accepted,
		"a ledger turn matching this request's role, provider, runner and timing — merged gap "+
			"content and all — IS this delivery genuinely landing; promptRecordAccepted must recognize "+
			"it as accepted instead of hashing the full recorded text against the journal's "+
			"original-text-only hash, which a merged delivery can never satisfy")
}
