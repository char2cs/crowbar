package chat_test

import (
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
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
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString())
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
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString())
	require.NoError(t, err)

	last := f.term.calls[len(f.term.calls)-1]
	assert.NotEqual(t, -1, indexOf(last.argv, text))
}
