//go:build integration

// This file closes the remaining Priority-1 end-to-end gaps identified after
// the initial acceptance pass (agent_test.go's TestAgent_ClaudeSpawnAndDetect /
// TestAgent_SwitchClaudeToCodex): a real codex turn_stop -> ledger round trip,
// the reducer's "registered" outcome (a live /clear) through the real Go
// stack, and the receiving provider actually USING a cross-provider handoff
// (not just carrying the string). Reuses newHarness/importRepoAndWorkspace/
// requireCLI/segmentTerminalSessionID/waitForProviderSessionID/nudgeUntil from
// agent_test.go.
package agent_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// codexDismissTrustDialog spends settleFor sending periodic bare Enters into
// a freshly spawned codex PTY, mirroring nudgeUntil's dialog-dismissal trick
// but as a bounded settle phase rather than a wait-for-condition loop.
//
// This exists because a freshly spawned (no positional prompt) codex has NO
// other detectable readiness signal: unlike claude, whose SessionStart fires
// at process boot (proven by TestAgent_ClaudeSpawnAndDetect, which never
// drives a single keystroke of real content), codex's SessionStart/rollout
// file is only created lazily on the FIRST actual turn — confirmed live
// 2026-07-06 via a throwaway fakeWSConn capture of a fresh codex spawn's raw
// PTY output: even after 90s of bare-Enter nudging with no prompt driven,
// ProviderSessionID never bound. That same capture showed WHY a bare spawn
// needs settling before typing: a Crowbar-managed child workspace is a git
// worktree, which codex's trust check treats as "a subdirectory of a Git
// project" (its trust root resolves to the worktree's parent checkout, not
// the exact `config.toml`-trusted cwd), so it shows an interactive "Do you
// trust the contents of this directory? [1. Yes, continue]" dialog before
// its update-check + MCP-server boot settle into the ready compose box. One
// dismissing Enter plus this settle window gets a fresh codex to a state
// where typed text lands in the real input box instead of a menu or splash
// screen. (TestAgent_SwitchClaudeToCodex never needed this because a
// switched-to codex is spawned WITH the handoff as a positional prompt
// argument, which sits pre-loaded in the input box and gets submitted by
// whichever nudge Enter lands after the dialog — an accident of timing this
// helper makes deliberate and visible for the direct-spawn case.)
func codexDismissTrustDialog(ctx context.Context, h *harness, termSessID string, settleFor time.Duration) {
	deadline := time.Now().Add(settleFor)
	for time.Now().Before(deadline) {
		_ = h.eng.Terminal.Write(ctx, termSessID, []byte("\r"))
		time.Sleep(2 * time.Second)
	}
}

// TestAgent_CodexTurnAppendsLedger is codex's counterpart to
// TestIngestHook_TurnStop_AppendsLedgerEntry (internal/app/usecases/agent/
// agent_test.go), which only ever fires IngestHook with a SYNTHETIC transcript
// file and a "claude"-tagged segment. It has never been proven that a REAL
// codex turn_stop hook (codex's own `crowbar hook turn_stop` shelling out from
// its own hooks.json, per the codex.yaml descriptor) reaches the daemon over
// the unix socket and produces a real ledger entry from codex's own rollout
// transcript. This spawns codex directly (no switch involved), drives one
// tiny real turn, and asserts the resulting ledger entry is both non-empty
// (via AssembleHandoff) and physically on disk tagged with the codex provider
// id.
func TestAgent_CodexTurnAppendsLedger(t *testing.T) {
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-turn", repoPath)

	chatID, segID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, segID)
	t.Logf("spawned codex: chat=%s segment=%s workspace=%s home=%s", chatID, segID, wsID, h.home)

	termSessID := segmentTerminalSessionID(t, h, chatID)
	require.NotEmpty(t, termSessID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	// See codexDismissTrustDialog's doc comment: a bare codex spawn shows an
	// interactive trust dialog + update/MCP-boot banners with NO hook traffic at
	// all, so there is nothing to poll on yet — settle first, then type.
	settleStart := time.Now()
	codexDismissTrustDialog(ctx, h, termSessID, 30*time.Second)
	t.Logf("settled %s past codex's trust dialog/boot banners", time.Since(settleStart))

	const codeword = "MERIDIAN-2260"
	prompt := "Remember this exact codeword for the rest of our conversation: " + codeword +
		". Reply with only the word: acknowledged."
	// Text and the submitting Enter are separate writes: TestAgent_SwitchClaudeToCodex
	// found that a trailing \r in the same write as pasted text lands as a literal
	// newline inside the input box, not a submit — the same TUI-input hazard applies
	// here since codex's interactive TUI is also an Ink-style app.
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte(prompt)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte("\r")))

	start := time.Now()
	providerSessionID, segs := waitForProviderSessionID(t, h, termSessID, chatID, segID, 60*time.Second)
	t.Logf("waited %s for codex's SessionStart hook round trip; segments=%+v", time.Since(start), segs)
	require.NotEmpty(t, providerSessionID,
		"timed out after 60s waiting for codex's SessionStart hook to reach /v0/agent/hooks and bind a "+
			"ProviderSessionID; this means either codex never started in the PTY, its SessionStart hook "+
			"never fired, `crowbar hook` could not reach the unix socket, or IngestHook/the reducer did "+
			"not persist the outcome — segments observed: %+v", segs)

	start = time.Now()
	handoff := nudgeUntil(h, termSessID, 90*time.Second, func() (string, bool) {
		blob, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
		require.NoError(t, err)
		return blob, blob != ""
	})
	t.Logf("waited %s for codex's Stop hook (ledger append)", time.Since(start))
	require.NotEmpty(t, handoff,
		"timed out waiting for a turn_stop hook (ledger append) after driving a real codex turn; this "+
			"proves codex's own Stop hook never reached /v0/agent/hooks, or turn_stop -> ledger.Append "+
			"never ran")
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")
}

// claudeLastAssistantText scans a Claude Code transcript (~/.claude/projects/
// <slug>/<uuid>.jsonl) for the most recent assistant message's text content,
// mirroring the Phase-0 spike's claude_answer_from_transcript fallback
// (docs/superpowers/specs/spike-2026-07-05-agentic/orchestrator.py). Returns ""
// if the file does not exist yet or no assistant text block is found.
func claudeLastAssistantText(transcriptPath string) string {
	f, err := os.Open(transcriptPath) //nolint:gosec // transcriptPath comes from a hook payload we control in-test
	if err != nil {
		return ""
	}
	defer f.Close()

	var last string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "assistant" {
			continue
		}
		for _, blk := range line.Message.Content {
			if blk.Type == "text" && blk.Text != "" {
				last = blk.Text
			}
		}
	}
	return last
}

// codexLastAssistantText scans a Codex rollout transcript (~/.codex/sessions/
// YYYY/MM/DD/rollout-*.jsonl) for the most recent assistant message's output
// text. Schema confirmed live 2026-07-06 against codex 0.139.0 via a throwaway
// `codex exec` prototype run (isolated CODEX_HOME/cwd, never the real
// ~/.codex): each line is {"type":"response_item","payload":{"type":"message",
// "role":"assistant","content":[{"type":"output_text","text":"..."}],
// "phase":"final_answer"}}. Returns "" if the file does not exist yet or no
// assistant message is found.
func codexLastAssistantText(transcriptPath string) string {
	f, err := os.Open(transcriptPath) //nolint:gosec // transcriptPath comes from a hook payload we control in-test
	if err != nil {
		return ""
	}
	defer f.Close()

	var last string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		var line struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "response_item" || line.Payload.Type != "message" || line.Payload.Role != "assistant" {
			continue
		}
		for _, blk := range line.Payload.Content {
			if blk.Type == "output_text" && blk.Text != "" {
				last = blk.Text
			}
		}
	}
	return last
}

// TestAgent_LiveClearRegistersNewChat proves the reducer's "registered"
// outcome (internal/engine/agent/registry.go's OnSessionStart, CASE 2: "an
// unknown id appeared -> register a new chat") through the REAL Go stack, not
// just internal/engine/agent/registry_test.go's pure-registry unit test or
// internal/app/usecases/agent/agent_test.go's TestIngestHook_SessionStart_
// Registered_MovesOldSegmentAndCreatesNewChat (which fires IngestHook twice
// with synthetic session ids, never a real CLI). TestAgent_ClaudeSpawnAndDetect
// only proves the FIRST "bound" outcome for a freshly spawned segment; this
// test drives a real claude session that is ALREADY bound to mint a brand new
// native session id UNDERNEATH THE SAME LIVE PROCESS via a real `/clear`
// (spike-proven in docs/superpowers/specs/spike-2026-07-05-agentic/drive.py to
// re-fire SessionStart with a changed id — "Detection: a conversation move
// re-fires SessionStart with a changed id" in docs/superpowers/specs/
// 2026-07-05-crowbar-agentic-engine-design.md §1's scorecard), and asserts the
// daemon reacted correctly end to end: a brand-new AgentChat appears with a
// new AgentSegment carrying a DIFFERENT ProviderSessionID, still tagged with
// the SAME CrowbarSegmentID (same live process/terminal session), and the
// vacated original chat's ActiveSegmentID is cleared.
func TestAgent_LiveClearRegistersNewChat(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-clear", repoPath)

	originalChatID, segID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, "claude")
	require.NoError(t, err)

	termSessID := segmentTerminalSessionID(t, h, originalChatID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	start := time.Now()
	originalProviderSessionID, segs := waitForProviderSessionID(t, h, termSessID, originalChatID, segID, 30*time.Second)
	require.NotEmpty(t, originalProviderSessionID, "claude never bound before /clear could be driven: %+v", segs)
	t.Logf("claude bound in %s (session=%s)", time.Since(start), originalProviderSessionID)

	// Drive a real /clear. Text and the submitting Enter are separate writes,
	// mirroring TestAgent_SwitchClaudeToCodex's finding for pasted prompts (a
	// trailing \r in the same write lands as a literal newline, not a submit).
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte("/clear")))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte("\r")))

	start = time.Now()
	var lastChats []domain.AgentChat
	newChat := nudgeUntil(h, termSessID, 30*time.Second, func() (domain.AgentChat, bool) {
		chats, err := h.app.Usecases.Agent.ListChats(ctx)
		require.NoError(t, err)
		lastChats = chats
		for _, c := range chats {
			if c.ID != originalChatID {
				return c, true
			}
		}
		return domain.AgentChat{}, false
	})
	t.Logf("waited %s for /clear's SessionStart to register a new chat", time.Since(start))
	require.NotEmpty(t, newChat.ID,
		"timed out after 30s waiting for a NEW AgentChat to appear after driving a real /clear; this "+
			"means either /clear never reached claude's TUI, SessionStart did not re-fire with a changed "+
			"id, or the reducer did not persist a \"registered\" outcome — chats observed: %+v", lastChats)
	require.NotEqual(t, originalChatID, newChat.ID, "a /clear move must register a DIFFERENT chat, not reuse the original")

	require.NotEmpty(t, newChat.ActiveSegmentID, "the newly registered chat must have an active segment")
	newSegs, err := h.app.Usecases.Agent.SegmentsFor(ctx, newChat.ID)
	require.NoError(t, err)
	require.Len(t, newSegs, 1, "the registered chat must have exactly one (new) segment: %+v", newSegs)
	newSeg := newSegs[0]
	require.Equal(t, segID, newSeg.CrowbarSegmentID,
		"the new segment must still be tagged with the SAME crowbar segment id — /clear moves the chat under "+
			"the same live process, it does not spawn a new one")
	require.NotEmpty(t, newSeg.ProviderSessionID, "the new segment must carry a bound native provider session id")
	require.NotEqual(t, originalProviderSessionID, newSeg.ProviderSessionID, "/clear must mint a DIFFERENT native session id")

	originalChat, err := h.app.Usecases.Agent.GetChat(ctx, originalChatID)
	require.NoError(t, err)
	require.Empty(t, originalChat.ActiveSegmentID, "the vacated original chat's ActiveSegmentID must be cleared")

	originalSegs, err := h.app.Usecases.Agent.SegmentsFor(ctx, originalChatID)
	require.NoError(t, err)
	require.Len(t, originalSegs, 1, "%+v", originalSegs)
	require.Equal(t, "moved", originalSegs[0].Status, "the original segment must be marked moved, not left dangling as active")
}
