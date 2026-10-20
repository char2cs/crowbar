package chat_test

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TestSubmitPrompt_MaterializesAttachmentsAndRewritesTheDispatchedMessage pins
// the wiring this task adds: a prompt referencing a durable attachment must
// have its dispatched argv carry the file's REAL, ABSOLUTE path in the
// durable store — never the logical chats/<id>/attachments/<file> reference a
// client-side markdown link encodes. Both Claude and Codex, at every
// permission level Crowbar offers, read an arbitrary absolute path outside
// their own worktree with no escalation and no prompt (confirmed live), so no
// copy step is needed to make that path readable.
func TestSubmitPrompt_MaterializesAttachmentsAndRewritesTheDispatchedMessage(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)

	text := "check this out ![photo](" + stored.Ref + ")"
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString(), "", nil)
	require.NoError(t, err)

	last := f.term.calls[len(f.term.calls)-1]
	absPath := filepath.ToSlash(filepath.Join(
		worktreepath.AttachmentsDir(f.ws.chatsDir, chatID), stored.FileName))
	wantText := "check this out ![photo](" + absPath + ")"
	assert.Equal(t, -1, indexOf(last.argv, text), "the raw durable ref must never reach the CLI's argv")
	assert.NotEqual(t, -1, indexOf(last.argv, wantText))
}

// TestSubmitPrompt_NoAttachmentReference_DispatchesTheTextUnchanged guards the
// zero-attachment fast path this wiring must not disturb: a plain prompt's
// argv must carry the exact text submitted, byte for byte.
func TestSubmitPrompt_NoAttachmentReference_DispatchesTheTextUnchanged(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	text := "just a plain message, no attachments"
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString(), "", nil)
	require.NoError(t, err)

	last := f.term.calls[len(f.term.calls)-1]
	assert.NotEqual(t, -1, indexOf(last.argv, text))
}

// A steered prompt reaches the CLI as its row records it for dispatch, whichever
// turn end hands it over: one whose commit failed leaves the next one the same
// text, and the user's own text stays theirs.
func TestSteer_TheTurnEndAfterAFailedOneHandsTheCLITheDispatchedText(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-steer")
	prompt(t, f, runnerID, "claude", "think hard about this")
	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)
	text, requestID := "and this ![photo]("+stored.Ref+")", uuid.NewString()
	steered, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, requestID, "", nil)
	require.NoError(t, err)
	require.Equal(t, runnerID, steered.RunnerID, "precondition: the prompt is steered into the running turn")
	turnStop := mustJSON(t, map[string]any{"last_assistant_message": "done"})

	require.NoError(t, f.ledgerDB.Exec(`CREATE TRIGGER refuse_turn_ended BEFORE INSERT ON chat_ledger
		WHEN NEW.kind = 'turn_ended' BEGIN SELECT RAISE(ABORT, 'boom: disk refused'); END`).Error)
	_, err = f.usecase.placeHook(f.ctx, engineagents.ChannelHooks, runnerID, "claude", "turn_stop", turnStop)
	require.Error(t, err, "a turn end the ledger refused is refused to its CLI")
	require.NoError(t, f.ledgerDB.Exec(`DROP TRIGGER refuse_turn_ended`).Error)
	reply, err := f.usecase.placeHook(f.ctx, engineagents.ChannelHooks, runnerID, "claude", "turn_stop", turnStop)
	require.NoError(t, err)

	absPath := filepath.ToSlash(filepath.Join(worktreepath.AttachmentsDir(f.ws.chatsDir, chatID), stored.FileName))
	assert.Contains(t, string(reply.Body), "("+absPath+")", "the CLI is handed the dispatched text")
	assert.NotContains(t, string(reply.Body), "("+stored.Ref+")")
	assert.Equal(t, text, receipt(t, f, chatID, requestID).Text, "the user's text is never the dispatched one")
}
