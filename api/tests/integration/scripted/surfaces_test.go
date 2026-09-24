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

func anyStrings(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, s := range list {
		str, _ := s.(string)
		out = append(out, str)
	}
	return out
}
