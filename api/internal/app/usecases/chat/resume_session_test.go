package chat_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

func TestResumeBySessionID_ARevivedChatResumesItsBoundSession(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-kept")
	exchange(t, f, runnerID, "claude", "kept content")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "sid-kept", argAfter(t, argv, "--resume"))
	snap, err := f.usecase.ChatSnapshot(f.ctx, chatID)
	require.NoError(t, err)
	c, ok := snap.Conversation()
	require.True(t, ok)
	assert.Equal(t, "sid-kept", c.SessionID)
}

func TestResumeBySessionID_ASessionThatNeverTurnedIsReplacedNotResumed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	spawned := argAfter(t, f.term.calls[0].argv, "--session-id")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	argv := f.term.calls[1].argv
	revived := argAfter(t, argv, "--session-id")
	assert.NotEqual(t, spawned, revived, "a revive never reuses an id its CLI never wrote")
	assert.NotContains(t, argv, "--resume", "nor resumes it")

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "first words", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[2].argv
	delivering := argAfter(t, argv, "--session-id")
	assert.NotContains(t, []string{spawned, revived}, delivering)
	assert.NotContains(t, argv, "--resume")
	c := f.conversation(t, chatID, "claude")
	assert.Equal(t, ledger.CauseAssigned, c.Cause)

	hook(t, f, first.RunnerID, "claude", "user_prompt", map[string]any{"prompt": "first words", "session_id": delivering})
	turn(t, f, first.RunnerID, "claude", "answered")
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "second words", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, delivering, argAfter(t, argv, "--resume"), "once it turned, it is resumed")
	assert.NotContains(t, argv, "--session-id")
}

func TestResumeBySessionID_AnUnturnedSessionOfAProviderThatTakesNoIDLaunchesFresh(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "thread-unturned")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	requestID := uuid.NewString()
	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello codex", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.NotContains(t, argv, "resume")
	assert.NotContains(t, argv, "thread-unturned")

	f.announce(t, result.RunnerID, "thread-fresh")
	assert.Equal(t, "thread-fresh", f.conversation(t, chatID, "codex").SessionID)
	assert.Empty(t, f.markers(t, chatID), "leaving a session that never turned is no change")
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the prompt this launch carries is still owed")

	prompt(t, f, result.RunnerID, "codex", "hello codex")
	assert.Equal(t, ledger.PromptStatusDelivered, receipt(t, f, chatID, requestID).Status)
}

func TestResumeBySessionID_ARefusedResumeLosesTheSession(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-corrupt")
	exchange(t, f, runnerID, "claude", "before the corruption")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	require.Equal(t, "sid-corrupt", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"))
	f.term.exitWith(t, f.runner(t, revived).TerminalSession, engineterminal.Exit{
		Code: 1, Screen: "No conversation found with session ID: sid-corrupt",
	})
	f.wait()

	assert.Equal(t, domain.AgentExitResumeFailed, agentusecase.ChatExitReason(f.usecase.RunnerUsecase, chatID))
	assert.Equal(t, ledger.ConversationLost, f.conversation(t, chatID, "claude").State)

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.NotContains(t, argv, "--resume", "a lost session is never resumed")
	fresh := f.conversation(t, chatID, "claude")
	assert.Equal(t, ledger.ConversationBound, fresh.State)
	assert.Equal(t, fresh.SessionID, argAfter(t, argv, "--session-id"))
	assert.Contains(t, strings.Join(argv, "\x00"), "before the corruption", "the conversation carries on from the record")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "after the loss", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[f.term.callCount()-1].argv
	assert.NotEqual(t, fresh.SessionID, argAfter(t, argv, "--session-id"), "the fresh session never turned")
	assert.Equal(t, ledger.CauseFresh, f.conversation(t, chatID, "claude").Cause, "and its replacement is fresh too")
	assert.Contains(t, strings.Join(argv, "\x00"), "after the loss")
	assert.Contains(t, strings.Join(argv, "\x00"), "before the corruption", "as does the launch carrying the prompt")
	assert.Equal(t, []string{ledger.MarkerFromRecord + ":"}, f.markers(t, chatID), "the transcript says so once, for the loss")
}

func TestResumeBySessionID_APromptCarriedIntoARefusedResumeIsLostWithIt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-corrupt")
	exchange(t, f, runnerID, "claude", "an answer")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	requestID := uuid.NewString()
	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "lost with it", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	require.Equal(t, "sid-corrupt", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"))
	calls := f.term.callCount()
	f.term.exitWith(t, result.TerminalSessionID, engineterminal.Exit{
		Code: 1, Screen: "No conversation found with session ID: sid-corrupt",
	})
	f.wait()

	got := receipt(t, f, chatID, requestID)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonSessionLost, got.Reason)
	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID))
	assert.Equal(t, calls, f.term.callCount(), "the lost prompt is not redelivered")
}

func TestResumeBySessionID_AResumeStoppedBeforeItSpeaksIsNotARefusal(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-kept")
	exchange(t, f, runnerID, "claude", "an answer")
	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	require.Equal(t, "sid-kept", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"))

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, result.TerminalSessionID)
	f.wait()
	assert.Equal(t, domain.AgentExitStopped, agentusecase.ChatExitReason(f.usecase.RunnerUsecase, chatID))

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	assert.Equal(t, "sid-kept", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"),
		"a session nobody refused is never lost")
}
