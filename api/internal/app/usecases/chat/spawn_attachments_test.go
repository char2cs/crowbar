package chat_test

import (
	"os"
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
// have its dispatched argv carry a REWRITTEN, worktree-relative path — never
// the durable chats/<id>/attachments/<file> reference a client-side markdown
// link encodes — and the referenced file must actually be copied into the
// runner's scratch dir under the worktree before the CLI is spawned.
func TestSubmitPrompt_MaterializesAttachmentsAndRewritesTheDispatchedMessage(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)

	text := "check this out ![photo](" + stored.Ref + ")"
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString())
	require.NoError(t, err)

	last := f.term.calls[len(f.term.calls)-1]
	wantText := "check this out ![photo](.crowbar-attachments/" + submission.RunnerID + "/" + stored.FileName + ")"
	assert.Equal(t, -1, indexOf(last.argv, text), "the raw durable ref must never reach the CLI's argv")
	assert.NotEqual(t, -1, indexOf(last.argv, wantText))

	data, err := os.ReadFile(filepath.Join(f.ws.worktree,
		worktreepath.AttachmentScratchDirName, submission.RunnerID, stored.FileName))
	require.NoError(t, err)
	assert.Equal(t, "bytes", string(data))
}

// TestSubmitPrompt_CleansUpTheScratchCopyOnNormalExit pins the other half:
// the scratch directory materialized for dispatch must not outlive the
// runner that owned it — onRunnerExit must reap it exactly the way it
// already reaps the per-runner tmp dir.
func TestSubmitPrompt_CleansUpTheScratchCopyOnNormalExit(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	stored, err := f.usecase.UploadAttachment(f.ctx, chatID, agentusecase.UploadAttachmentInput{
		ID: "ab12", OriginalName: "photo.png", Data: []byte("bytes"),
	})
	require.NoError(t, err)
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, "![photo]("+stored.Ref+")", uuid.NewString())
	require.NoError(t, err)
	scratchDir := worktreepath.AttachmentScratchDir(f.ws.worktree, submission.RunnerID)
	require.DirExists(t, scratchDir)

	f.term.exit(t, submission.TerminalSessionID)

	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr), "scratch attachment dir must be removed on normal exit")
}

// TestSubmitPrompt_NoAttachmentReference_DispatchesTheTextUnchanged guards the
// zero-attachment fast path this wiring must not disturb: a plain prompt's
// argv must carry the exact text submitted, byte for byte, with no scratch
// directory created for a runner that never referenced a file.
func TestSubmitPrompt_NoAttachmentReference_DispatchesTheTextUnchanged(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	text := "just a plain message, no attachments"
	submission, err := f.usecase.SubmitPrompt(f.ctx, chatID, text, uuid.NewString())
	require.NoError(t, err)

	last := f.term.calls[len(f.term.calls)-1]
	assert.NotEqual(t, -1, indexOf(last.argv, text))

	scratchDir := worktreepath.AttachmentScratchDir(f.ws.worktree, submission.RunnerID)
	assert.NoDirExists(t, scratchDir, "a dispatch with no attachment reference must create no scratch dir")
}
