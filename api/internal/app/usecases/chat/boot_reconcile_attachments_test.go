package chat_test

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// TestReconcileRunnersOnBoot_ReapsOrphanedScratchAttachments pins the other
// half of the dual-cleanup mechanism: onRunnerExit reaps a runner's scratch
// attachment dir on a normal exit, but a daemon crash or restart means every
// PTY dies with no onExit callback ever firing, so that dir would otherwise
// accumulate forever. Boot reconciliation is the only remaining place that
// can still reap it.
func TestReconcileRunnersOnBoot_ReapsOrphanedScratchAttachments(t *testing.T) {
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

	// The daemon restarts: every PTY dies with no onExit callback ever firing.
	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr), "boot reconciliation must reap an orphaned scratch attachment dir")
}

// TestReconcileRunnersOnBoot_ReapAttachments_WorkspaceLookupFailure_SkipsGracefully
// pins the best-effort guard: a runner whose workspace can no longer be
// resolved (its worktree lookup fails) must be skipped — logged, not
// fatal — leaving the rest of boot reconciliation to run to completion.
func TestReconcileRunnersOnBoot_ReapAttachments_WorkspaceLookupFailure_SkipsGracefully(t *testing.T) {
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

	f.term.dieWithDaemon()
	f.ws.worktreeErr = errors.New("boom: worktree lookup for boot reap")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx),
		"a workspace lookup failure must not fail the boot sequence")

	require.DirExists(t, scratchDir, "a runner whose workspace lookup fails must be skipped, not reaped")
}

// TestReconcileRunnersOnBoot_NoScratchDir_IsANoOp covers the common case: a
// runner that never dispatched a prompt referencing an attachment has no
// scratch dir to begin with, so reaping it is a cheap no-op that must not
// disturb the rest of boot reconciliation.
func TestReconcileRunnersOnBoot_NoScratchDir_IsANoOp(t *testing.T) {
	f := newFixture(t)
	_, runnerID := f.spawn(t, "claude")
	scratchDir := worktreepath.AttachmentScratchDir(f.ws.worktree, runnerID)
	require.NoDirExists(t, scratchDir, "precondition: no attachment was ever dispatched")

	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

	assert.NoDirExists(t, scratchDir)
}
