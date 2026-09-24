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

// TestRegression_ClassifyPriorAttempt_APTYLessDeliveryReplaysAsTheSameSuccess
// is the second half of the orphan-PTY fix's fallout on the durable journal
// (see commitPromptSpawn's own regression test): MarkSpawned now legitimately
// records an EMPTY terminalSessionId for an api-driven runner, and the
// short-circuit that makes a retried client request id replay as the SAME
// success required a non-empty one.
//
// Without this, an ordinary retry (the client lost the response to a network
// hiccup) stopped being idempotent and fell through to the ledger-derived
// recovery instead, answering ErrPromptOutcomeUnknown or
// ErrPromptAlreadyAccepted for a delivery the journal had already confirmed.
//
// State is what actually proves the dispatch committed — Begin writes
// RunnerID while still `dispatching`, and only MarkSpawned advances it — so
// the terminal session was never carrying that meaning in the first place.
func TestRegression_ClassifyPriorAttempt_APTYLessDeliveryReplaysAsTheSameSuccess(t *testing.T) {
	// An empty ledger, so falling through to the recovery path is a visible
	// ErrPromptOutcomeUnknown rather than an accidental pass.
	rs := &Runners{conversations: stubConversationsWithTurn{}}

	got, done, err := rs.classifyPriorAttempt(context.Background(), domain.Chat{ID: "chat-1"}, "", "req-1",
		agentjournal.PromptRequest{
			RequestID: "req-1",
			RunnerID:  "runner-1",
			State:     agentjournal.PromptStateSpawned,
			// No PTY: this runner's api connection IS its process.
			TerminalSessionID: "",
		})

	require.True(t, done)
	require.NoError(t, err)
	assert.Equal(t, "runner-1", got.RunnerID)
}

// A record that never reached `spawned` is NOT a completed delivery, and must
// still take the ledger-derived recovery path rather than replaying as one.
func TestClassifyPriorAttempt_ADispatchingRecordStillRecovers(t *testing.T) {
	rs := &Runners{conversations: stubConversationsWithTurn{}}

	_, done, err := rs.classifyPriorAttempt(context.Background(), domain.Chat{ID: "chat-1"}, "", "req-1",
		agentjournal.PromptRequest{
			RequestID: "req-1", RunnerID: "runner-1", State: agentjournal.PromptStateDispatching,
		})

	require.True(t, done)
	require.ErrorIs(t, err, ErrPromptOutcomeUnknown)
}

// An `accepted` record whose MarkSpawned never ran committed no delivery
// identity at all — the user_prompt hook advanced it straight out of
// `dispatching` while commitPromptSpawn was still failing. It must recover
// from the ledger, never replay as a success DTO naming a delivery that was
// never recorded. Pins the narrow half of the relaxation above.
func TestClassifyPriorAttempt_AnAcceptedRecordWithNoCommittedIdentityStillRecovers(t *testing.T) {
	rs := &Runners{conversations: stubConversationsWithTurn{}}

	_, done, err := rs.classifyPriorAttempt(context.Background(), domain.Chat{ID: "chat-1"}, "", "req-1",
		agentjournal.PromptRequest{
			RequestID: "req-1", RunnerID: "runner-1",
			State: agentjournal.PromptStateAccepted, TerminalSessionID: "",
		})

	require.True(t, done)
	require.ErrorIs(t, err, ErrPromptAlreadyAccepted)
}
