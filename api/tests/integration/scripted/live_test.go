//go:build integration && unix

package scripted_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// The same lifecycle, against the REAL claude and codex under their shipped
// descriptors, a local stand-in model answering their turns. Opt-in
// (SCRIPTED_LIVE=1): it needs both CLIs installed, and nothing else.

func newLiveRig(t *testing.T) (*rig, *modelServer) {
	t.Helper()
	if os.Getenv("SCRIPTED_LIVE") == "" {
		t.Skip("set SCRIPTED_LIVE=1 to run the real CLIs against the stand-in model")
	}
	for _, cli := range providers {
		if _, err := exec.LookPath(cli); err != nil {
			t.Skipf("%s is not installed", cli)
		}
	}
	kit.RequireNoLeakedProcesses(t)
	model := newModelServer(t)
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	kit.IsolateProviderHomes(t)
	t.Setenv("ANTHROPIC_BASE_URL", model.URL)
	t.Setenv("ANTHROPIC_API_KEY", liveAPIKey)
	t.Setenv("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1")
	t.Setenv("DISABLE_TELEMETRY", "1")
	t.Setenv("LIVE_OPENAI_KEY", "live")
	require.NoError(t, os.WriteFile(filepath.Join(os.Getenv("CODEX_HOME"), "config.toml"), []byte(
		"model = \"gpt-live\"\nmodel_provider = \"live\"\n[model_providers.live]\nname = \"live\"\n"+
			"base_url = \""+model.URL+"/v1\"\nwire_api = \"responses\"\nenv_key = \"LIVE_OPENAI_KEY\"\n"), 0o600))
	scratch := t.TempDir()
	r := &rig{t: t, home: home, script: filepath.Join(scratch, "unused.yaml"), log: filepath.Join(scratch, "unused.log")}
	r.d = bootDaemon(t, home)
	t.Cleanup(func() { r.d.stop() })
	r.wsID = r.workspace(kit.InitRepo(t))
	trustForClaude(t, r)
	return r, model
}

const liveAPIKey = "sk-ant-api03-livelivelivelivelivelivelive"

// trustForClaude records what a person set up once: onboarding done, the API
// key approved, the worktree trusted.
func trustForClaude(t *testing.T, r *rig) {
	t.Helper()
	path := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".claude.json")
	cfg := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &cfg)
	}
	_, _, _, ws, err := r.d.app.Usecases.AgentWorkspaceReader.WorktreeDir(context.Background(), r.wsID)
	require.NoError(t, err)
	cfg["hasCompletedOnboarding"] = true
	cfg["customApiKeyResponses"] = map[string]any{"approved": []string{liveAPIKey[len(liveAPIKey)-20:]}, "rejected": []string{}}
	cfg["projects"] = map[string]any{ws: map[string]any{"hasTrustDialogAccepted": true, "hasCompletedProjectOnboarding": true}}
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func (r *rig) replied(chatID string, n int) func() bool {
	return func() bool {
		said := r.said(chatID)
		count := 0
		for _, s := range said {
			if s == "assistant: "+liveReply {
				count++
			}
		}
		return count >= n && !r.working(chatID)
	}
}

func (r *rig) sessions(chatID string) []string {
	r.t.Helper()
	convs, err := r.d.app.Usecases.AgentRunner.ConversationsForChat(context.Background(), chatID)
	require.NoError(r.t, err)
	var out []string
	for _, c := range convs {
		out = append(out, c.SessionID)
	}
	return out
}

// A real turn lands; the next message continues the same vendor session;
// a stopped chat resumes it.
func TestLive_TurnsContinueAndResumeTheSameSession(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r, _ := newLiveRig(t)
			chatID := r.spawn(provider)
			require.NoError(t, r.send(chatID, "first"))
			r.eventually(r.replied(chatID, 1), "the first real turn never landed")
			require.NoError(t, r.send(chatID, "second"))
			r.eventually(r.replied(chatID, 2), "the second real turn never landed")
			require.Len(t, r.sessions(chatID), 1, "both turns on one vendor session")

			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)
			require.NoError(t, r.send(chatID, "after a stop"))
			r.eventually(r.replied(chatID, 3), "the revived real CLI never answered")

			assert.Len(t, r.sessions(chatID), 1, "the revive resumed the same vendor session")
			assert.Equal(t, domain.AgentRungSession, r.snapshot(chatID).Session.Rung)
			assert.True(t, slices.Contains(r.said(chatID), "user: after a stop"), "%q\nhooks:\n%s", r.said(chatID), r.d.hooks)
		})
	}
}

// A daemon restart while the real CLI waits on its model closes the turn,
// says why, and the next message resumes the same vendor session.
func TestLive_DaemonRestartMidTurn(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r, model := newLiveRig(t)
			chatID := r.spawn(provider)
			require.NoError(t, r.send(chatID, "first"))
			r.eventually(r.replied(chatID, 1), "the first real turn never landed")
			model.holdTurns()
			require.NoError(t, r.send(chatID, "caught by a restart"))
			r.eventually(func() bool { return r.working(chatID) }, "the held turn never opened")

			r.restart()
			model.release()

			r.dormantSaying(chatID, domain.AgentExitDaemonRestart)
			require.NoError(t, r.send(chatID, "after the restart"))
			r.eventually(r.replied(chatID, 2), "the chat did not continue after a restart")
			assert.Len(t, r.sessions(chatID), 1, "the same vendor session continued")
		})
	}
}

// The real codex opens its TUI before any turn wrote a rollout, takes a turn
// there, and the chat surface resumes the session the TUI started.
func TestLive_CodexTerminalBeforeAnyTurnAndBack(t *testing.T) {
	r, _ := newLiveRig(t)
	ctx := context.Background()
	chatID := r.spawn("codex")

	_, err := r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	require.NoError(t, r.send(chatID, "in the terminal"))
	r.eventually(r.replied(chatID, 1), "the TUI never answered")
	before := r.sessions(chatID)

	require.NoError(t, r.runners().SwitchToNative(ctx, chatID))
	require.NoError(t, r.send(chatID, "back in the chat"))
	r.eventually(r.replied(chatID, 2), "the chat surface never answered")

	assert.Equal(t, before, r.sessions(chatID), "the chat surface resumed the TUI's session, starting none")
	assert.Equal(t, domain.AgentRungSession, r.snapshot(chatID).Session.Rung)
}

// A real codex chat born in its TUI moves to Crowbar's chat on the same
// session, and claude's one process moves between surfaces in place.
func TestLive_TerminalChatsMoveToTheChat(t *testing.T) {
	r, _ := newLiveRig(t)
	ctx := context.Background()
	codexChat, err := r.d.app.Usecases.AgentChat.MintChat(ctx, r.wsID, "codex", "terminal")
	require.NoError(t, err)
	_, err = r.runners().StartRunner(ctx, codexChat, "codex")
	require.NoError(t, err)
	require.NoError(t, r.send(codexChat, "in the terminal"))
	r.eventually(r.replied(codexChat, 1), "codex's TUI never answered")
	before := r.sessions(codexChat)

	require.NoError(t, r.runners().SwitchToNative(ctx, codexChat))
	require.NoError(t, r.send(codexChat, "now in the chat"))
	r.eventually(r.replied(codexChat, 2), "the chat surface never answered")
	assert.Equal(t, before, r.sessions(codexChat), "the chat surface resumed the TUI's session")

	claudeChat := r.spawn("claude")
	_, err = r.runners().SwitchToTerminal(ctx, claudeChat)
	require.NoError(t, err)
	require.NoError(t, r.send(claudeChat, "in claude's terminal"))
	r.eventually(r.replied(claudeChat, 1), "claude never answered on its terminal")
	require.NoError(t, r.runners().SwitchToNative(ctx, claudeChat))
	require.NoError(t, r.send(claudeChat, "in claude's chat"))
	r.eventually(r.replied(claudeChat, 2), "claude never answered on the chat")
	assert.Len(t, r.sessions(claudeChat), 1)
}

// A real claude chat switched to codex opens codex's TUI and answers there;
// switching back resumes claude's own session.
func TestLive_SwitchToCodexThenItsTerminal(t *testing.T) {
	r, _ := newLiveRig(t)
	ctx := context.Background()
	chatID := r.spawn("claude")
	require.NoError(t, r.send(chatID, "first"))
	r.eventually(r.replied(chatID, 1), "claude never answered")

	_, err := r.runners().SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	_, err = r.runners().SwitchToTerminal(ctx, chatID)
	require.NoError(t, err)
	require.NoError(t, r.send(chatID, "codex in its terminal"))
	r.eventually(r.replied(chatID, 2), "codex's TUI never answered")

	_, err = r.runners().SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	require.NoError(t, r.send(chatID, "claude again"))
	r.eventually(r.replied(chatID, 3), "claude never answered again")
	assert.Equal(t, domain.AgentRungSession, r.snapshot(chatID).Session.Rung, "claude resumed its own session")
}

// A vendor session the CLI no longer has continues on a fresh one handed the
// transcript — with the real CLIs, which exit on an unknown session.
func TestLive_LostSessionContinuesFromTheTranscript(t *testing.T) {
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			r, _ := newLiveRig(t)
			chatID := r.spawn(provider)
			require.NoError(t, r.send(chatID, "first"))
			r.eventually(r.replied(chatID, 1), "the first real turn never landed")
			require.NoError(t, r.runners().StopChat(context.Background(), chatID))
			r.dormantSaying(chatID, domain.AgentExitStopped)
			removeSessionFiles(t)

			require.NoError(t, r.send(chatID, "still there?"))
			r.eventually(r.replied(chatID, 2), "a lost session stranded the chat")
			assert.Equal(t, domain.AgentRungTranscript, r.snapshot(chatID).Session.Rung)
			assert.Len(t, r.sessions(chatID), 2, "a new vendor session carries the chat on: %v\nhooks:\n%s", r.said(chatID), r.d.hooks)
		})
	}
}
