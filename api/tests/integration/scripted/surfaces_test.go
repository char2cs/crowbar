//go:build integration && unix

package scripted_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// codex on the terminal surface is its TUI in a PTY, reporting over hooks:
// it boots past every modal (trust, resume cwd are pre-answered), announces
// its session on the first turn, and the next message resumes it.
func TestScripted_CodexTerminalSurfaceBootsAndResumes(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID, err := r.d.app.Usecases.AgentChat.MintChat(ctx, r.wsID, "codex", "terminal")
	require.NoError(t, err)
	_, err = r.runners().StartRunner(ctx, chatID, "codex")
	require.NoError(t, err)

	r.eventually(func() bool { return len(r.logged("start")) > 0 }, "the TUI never started")
	boot := r.logged("start")
	argv := strings.Join(anyStrings(boot[0]["argv"]), "\x00")
	assert.Contains(t, argv, `trust_level="trusted"`, "folder trust is pre-answered")
	assert.Contains(t, argv, `tui.resume_cwd="current"`, "the resume-cwd modal is pre-answered")
	assert.Contains(t, argv, "hooks.SessionStart=", "the TUI reports over hooks")
	assert.Nil(t, boot[0]["serve"], "no app-server on the terminal surface: one channel per process")

	r.firstTurn(chatID)
	require.NoError(t, r.send(chatID, "and again"))
	r.eventually(func() bool { return len(r.said(chatID)) == 4 && !r.working(chatID) }, "second turn")

	prompts := r.logged("prompt")
	assert.Equal(t, prompts[0]["session"], prompts[len(prompts)-1]["session"], "the TUI resumed its own session")
	assert.Empty(t, r.logged("refused"))
}

// "Open in terminal" hands a native codex chat to its TUI and back: one
// channel at a time, the same thread throughout.
func TestScripted_CodexOpenInTerminalAndBack(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("codex")
	r.firstTurn(chatID)
	thread := r.logged("prompt")[0]["session"]

	_, err := r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	r.eventually(func() bool {
		for _, e := range r.logged("start") {
			if e["resume"] == thread {
				return true
			}
		}
		return false
	}, "the TUI never attached to the chat's thread")

	require.NoError(t, r.runners().SwitchToNative(ctx, chatID))
	r.continues(chatID, "back in the chat")

	prompts := r.logged("prompt")
	assert.Equal(t, thread, prompts[len(prompts)-1]["session"], "the chat is still on its thread")
	assert.True(t, slices.ContainsFunc(r.logged("rpc"), func(e logEntry) bool { return e["method"] == "thread/resume" }),
		"coming back re-established the connection on that thread")
	assert.Equal(t, "live", r.snapshot(chatID).Phase)
	assert.NotEqual(t, domain.AgentExitResumeFailed, r.snapshot(chatID).Session.ExitReason)
}

// Stopping a chat whose native view is open ends the view and the runner,
// within a bound, and the chat takes the next message.
func TestScripted_CodexStopWhileInTerminal(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("codex")
	r.firstTurn(chatID)
	_, err := r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- r.runners().StopChat(ctx, chatID) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("Stop with the native view open never returned (S4)")
	}
	r.dormantSaying(chatID, domain.AgentExitStopped)
	r.continues(chatID, "after stopping the terminal")
}

// Opening codex's TUI before any turn completed starts it fresh — there is
// no rollout to resume — and the conversation carries on there and back.
func TestScripted_CodexOpenInTerminalBeforeAnyTurn(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("codex")

	term, err := r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	assert.NotEmpty(t, term, "the TUI's own terminal session")
	r.eventually(func() bool { return len(r.codexTUIStarts()) == 1 }, "the TUI never started")
	assert.Empty(t, r.codexTUIStarts()[0]["resume"], "nothing to resume yet")
	assert.Equal(t, domain.SurfaceTerminal, r.snapshot(chatID).Chat.Surface)
	r.continues(chatID, "in the terminal")
	tuiSession := r.lastPrompt()["session"]

	require.NoError(t, r.runners().SwitchToNative(ctx, chatID))
	r.continues(chatID, "back in the chat")

	assert.Equal(t, domain.SurfaceChat, r.snapshot(chatID).Chat.Surface)
	assert.True(t, r.called("thread/resume", tuiSession), "the chat resumed the TUI's session")
	assert.Equal(t, tuiSession, r.lastPrompt()["session"])
	assert.Empty(t, r.logged("refused"))
}

// A claude chat switched to codex opens codex's TUI on Crowbar's transcript,
// and switching back to claude there resumes claude's own session.
func TestScripted_SwitchProviderThenOpenTheTerminal(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("claude")
	r.firstTurn(chatID)
	claudeSession := r.lastPrompt()["session"]

	_, err := r.runners().SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	_, err = r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	r.eventually(func() bool { return len(r.codexTUIStarts()) == 1 }, "codex's TUI never started")
	assert.Contains(t, strings.Join(anyStrings(r.codexTUIStarts()[0]["argv"]), " "), "PELICAN",
		"codex's TUI was handed the conversation")
	r.continues(chatID, "codex in its terminal")

	_, err = r.runners().SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	r.continues(chatID, "claude again")
	assert.Equal(t, "claude", r.lastPrompt()["cli"])
	assert.Equal(t, claudeSession, r.lastPrompt()["session"], "claude resumed its own session")
	assert.Equal(t, domain.SurfaceTerminal, r.snapshot(chatID).Chat.Surface)
}

// A codex chat born in its TUI moves to Crowbar's chat on the same session.
func TestScripted_CodexTerminalChatMovesToTheChat(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID, err := r.d.app.Usecases.AgentChat.MintChat(ctx, r.wsID, "codex", "terminal")
	require.NoError(t, err)
	_, err = r.runners().StartRunner(ctx, chatID, "codex")
	require.NoError(t, err)
	r.firstTurn(chatID)
	tuiSession := r.lastPrompt()["session"]

	require.NoError(t, r.runners().SwitchToNative(ctx, chatID))
	r.continues(chatID, "now in the chat")

	assert.Equal(t, domain.SurfaceChat, r.snapshot(chatID).Chat.Surface)
	assert.True(t, r.called("thread/resume", tuiSession), "the app-server resumed the TUI's session")
	assert.Equal(t, tuiSession, r.lastPrompt()["session"])
}

// A dormant chat moves surface without starting anything; its next message
// starts it there.
func TestScripted_ADormantChatStartsOnTheSurfaceItMovedTo(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("codex")
	r.firstTurn(chatID)
	thread := r.lastPrompt()["session"]
	require.NoError(t, r.runners().StopChat(ctx, chatID))
	r.dormantSaying(chatID, domain.AgentExitStopped)

	_, err := r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	live, _ := r.live(chatID)
	assert.False(t, live, "moving a dormant chat starts nothing")
	assert.Equal(t, domain.SurfaceTerminal, r.snapshot(chatID).Chat.Surface)

	r.continues(chatID, "in the terminal now")
	require.NotEmpty(t, r.codexTUIStarts(), "the revive did not start the TUI")
	for _, start := range r.codexTUIStarts() {
		assert.Equal(t, thread, start["resume"], "the TUI resumed the chat's thread")
	}
	assert.Equal(t, thread, r.lastPrompt()["session"])
}

// claude's one process serves both surfaces: moving between them starts nothing.
func TestScripted_ClaudeMovesSurfacesOnItsOwnProcess(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	chatID := r.spawn("claude")
	r.firstTurn(chatID)
	_, runnerID := r.live(chatID)
	starts := len(r.logged("start"))

	term, err := r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	live, err := r.runners().LiveRunnerForChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, live.TerminalSession, term)
	assert.Equal(t, domain.SurfaceTerminal, r.snapshot(chatID).Chat.Surface)
	require.NoError(t, r.runners().SwitchToNative(ctx, chatID))
	assert.Equal(t, domain.SurfaceChat, r.snapshot(chatID).Chat.Surface)

	_, still := r.live(chatID)
	assert.Equal(t, runnerID, still)
	assert.Len(t, r.logged("start"), starts)
}

// codexTUIStarts is every launch of codex's TUI (not its app-server).
func (r *rig) codexTUIStarts() []logEntry {
	var out []logEntry
	for _, e := range r.logged("start") {
		if e["cli"] == "codex" && e["serve"] == nil {
			out = append(out, e)
		}
	}
	return out
}

func (r *rig) lastPrompt() logEntry {
	r.t.Helper()
	prompts := r.logged("prompt")
	require.NotEmpty(r.t, prompts)
	return prompts[len(prompts)-1]
}

// called reports whether the app-server was sent method for thread.
func (r *rig) called(method string, thread any) bool {
	return slices.ContainsFunc(r.logged("rpc"), func(e logEntry) bool {
		return e["method"] == method && e["thread"] == thread
	})
}

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, s := range list {
		str, _ := s.(string)
		out = append(out, str)
	}
	return out
}
