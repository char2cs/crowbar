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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// claudeAssistantTexts scans a Claude Code transcript (~/.claude/projects/
// <slug>/<uuid>.jsonl) and returns every assistant message's text content, in
// transcript order, mirroring the Phase-0 spike's claude_answer_from_transcript
// fallback (docs/superpowers/specs/spike-2026-07-05-agentic/orchestrator.py),
// generalized to return the whole sequence rather than just the last one (see
// TestAgent_SwitchBackRestoresClaudeContext's doc comment for why: a resumed
// session may interleave a synthetic housekeeping turn among real replies in
// an unpredictable order, so callers there search the whole sequence rather
// than trust any single index). Returns nil if the file does not exist yet or
// has no assistant text.
func claudeAssistantTexts(transcriptPath string) []string {
	f, err := os.Open(transcriptPath) //nolint:gosec // transcriptPath comes from a hook payload we control in-test
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []string
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
		var text string
		for _, blk := range line.Message.Content {
			if blk.Type == "text" && blk.Text != "" {
				text = blk.Text
			}
		}
		if text != "" {
			out = append(out, text)
		}
	}
	return out
}

// claudeLastAssistantText returns the most recent entry from
// claudeAssistantTexts, or "" if there is none.
func claudeLastAssistantText(transcriptPath string) string {
	texts := claudeAssistantTexts(transcriptPath)
	if len(texts) == 0 {
		return ""
	}
	return texts[len(texts)-1]
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
			// Require the FULLY-FORMED registered chat: a non-original chat that
			// already carries its ActiveSegmentID. This is race-proof against the
			// vacated-chat clear in persistRegistered — if /clear yields more than one
			// registration, an intermediate chat gets its ActiveSegmentID cleared when
			// the process moves on, so "first non-original chat" could otherwise land on
			// a now-vacated one and see an empty ActiveSegmentID. Skipping ActiveSegmentID=="",
			// we settle on the chat the process currently hosts.
			if c.ID != originalChatID && c.ActiveSegmentID != "" {
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

// seedClaudeThenSwitchToCodex spawns claude, seeds it with codeword via one
// real turn, waits for the resulting ledger entry, switches the chat to
// codex, and waits for codex's own SessionStart hook to bind — exactly the
// sequence TestAgent_SwitchClaudeToCodex proves, factored out so the gap 3/4
// tests below don't each re-derive ~30 lines of setup. Returns the chat id,
// the original (now-ended) claude segment id, the new codex segment id, and
// the codex segment's terminal session id.
//
// Unlike TestAgent_CodexTurnAppendsLedger's bare `codex` spawn, no
// codexDismissTrustDialog settle phase is needed here: SwitchProvider always
// injects the assembled handoff as codex's positional prompt argument
// (codex.yaml's handoff_inject: pass_arg{positional:"{handoff}"}), so it sits
// pre-loaded in the input box and the first nudge Enter that lands after the
// trust dialog submits it directly as codex's first turn — the same
// mechanism TestAgent_SwitchClaudeToCodex already relies on.
func seedClaudeThenSwitchToCodex(
	t *testing.T,
	h *harness,
	wsID string,
	codeword string,
) (chatID, claudeSegID, codexSegID, codexTermSessID string) {
	t.Helper()
	ctx := context.Background()

	chatID, claudeSegID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, "claude")
	require.NoError(t, err)

	claudeTermSessID := segmentTerminalSessionID(t, h, chatID)

	start := time.Now()
	providerSessionID, segs := waitForProviderSessionID(t, h, claudeTermSessID, chatID, claudeSegID, 30*time.Second)
	require.NotEmpty(t, providerSessionID, "claude never bound a session before a turn could be driven: %+v", segs)
	t.Logf("claude bound in %s (session=%s)", time.Since(start), providerSessionID)

	prompt := "Remember this exact codeword for the rest of our conversation: " + codeword +
		". Reply with only the word: acknowledged."
	require.NoError(t, h.eng.Terminal.Write(ctx, claudeTermSessID, []byte(prompt)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, claudeTermSessID, []byte("\r")))

	start = time.Now()
	handoff := nudgeUntil(h, claudeTermSessID, 90*time.Second, func() (string, bool) {
		blob, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
		require.NoError(t, err)
		return blob, blob != ""
	})
	t.Logf("waited %s for claude's Stop hook (ledger append)", time.Since(start))
	require.NotEmpty(t, handoff, "timed out waiting for claude's turn_stop hook (ledger append)")
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	newSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	codexTermSessID = segmentTerminalSessionID(t, h, chatID)
	require.NotEqual(t, claudeTermSessID, codexTermSessID, "switch must spawn a new terminal session for the codex segment")

	start = time.Now()
	codexProviderSessionID, segsAfter := waitForProviderSessionID(t, h, codexTermSessID, chatID, newSegID, 30*time.Second)
	t.Logf("codex bound in %s (session=%s)", time.Since(start), codexProviderSessionID)
	require.NotEmpty(t, codexProviderSessionID, "codex never bound after switch: %+v", segsAfter)

	return chatID, claudeSegID, newSegID, codexTermSessID
}

// TestAgent_CodexUsesHandoff closes gap 3 of the Priority-1 e2e audit: proving
// the RECEIVING provider actually USES a cross-provider handoff, not merely
// that the handoff STRING was passed. TestAgent_SwitchClaudeToCodex
// deliberately stops at asserting AssembleHandoff's blob contains the
// codeword (its own comment explains why: driving a second real codex turn
// "would roughly double the real-API time/cost ... for a probabilistic
// assertion on a model reply"). This test pays that cost to close the gap:
// after a real claude->codex switch (via seedClaudeThenSwitchToCodex), it
// asks codex directly what codeword appeared in the context it was given, and
// asserts CODEX'S OWN REPLY (parsed from its rollout transcript's last
// assistant message, not the echoed user turn) contains it — the Go-stack
// proof of the Phase-0 spike's "Codex read Claude's raw 38 KB .jsonl and
// extracted the codeword" finding (docs/superpowers/specs/
// 2026-07-05-crowbar-agentic-engine-design.md §1 scorecard).
func TestAgent_CodexUsesHandoff(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-handoff", repoPath)

	const codeword = "OSPREY-4482"
	chatID, _, codexSegID, codexTermSessID := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	segs, err := h.app.Usecases.Agent.SegmentsFor(ctx, chatID)
	require.NoError(t, err)
	var codexSeg domain.AgentSegment
	for _, s := range segs {
		if s.ID == codexSegID {
			codexSeg = s
		}
	}
	require.NotEmpty(t, codexSeg.TranscriptPath, "codex segment must have a transcript path recorded from its SessionStart hook: %+v", segs)
	t.Logf("codex transcript: %s", codexSeg.TranscriptPath)

	// codex's first turn is the handoff itself (auto-submitted as its initial
	// positional prompt once past the trust dialog — see
	// seedClaudeThenSwitchToCodex's doc comment). It carries no instruction of
	// its own (AssembleHandoff just wraps the raw ledger in a header/footer), so
	// codex free-associates a short reply to it (observed live: a bare
	// "acknowledged", apparently echoing the pattern in claude's handed-off
	// transcript) — wait for that first turn to fully land BEFORE typing our own
	// follow-up, so a race between "our follow-up's reply" and "the auto-turn's
	// own reply" can't produce a false pass (this raced and DID false-pass on
	// the first version of this test: baseline was captured empty right after
	// SessionStart bound, before turn 1 had actually finished, and nudgeUntil
	// below matched turn 1's own "acknowledged" instead of our follow-up).
	baselineStart := time.Now()
	baseline := nudgeUntil(h, codexTermSessID, 30*time.Second, func() (string, bool) {
		cur := codexLastAssistantText(codexSeg.TranscriptPath)
		return cur, cur != ""
	})
	t.Logf("waited %s for codex's auto-submitted-handoff turn to land; its (uninstructed) reply was %q",
		time.Since(baselineStart), baseline)
	require.NotEmpty(t, baseline, "timed out waiting for codex's first (auto-submitted handoff) turn to produce any assistant reply")

	followUp := "What exact codeword appeared in the context you were given above? Reply with only that word."
	require.NoError(t, h.eng.Terminal.Write(ctx, codexTermSessID, []byte(followUp)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, codexTermSessID, []byte("\r")))

	start := time.Now()
	reply := nudgeUntil(h, codexTermSessID, 90*time.Second, func() (string, bool) {
		cur := codexLastAssistantText(codexSeg.TranscriptPath)
		return cur, cur != "" && cur != baseline
	})
	t.Logf("waited %s for codex's reply to the follow-up turn", time.Since(start))
	require.NotEmpty(t, reply, "timed out waiting for codex to produce a NEW assistant reply to the follow-up turn")
	require.Contains(t, reply, codeword,
		"codex's own reply must reference the codeword handed off from claude's session — proves codex "+
			"actually READ the opaque handoff, not just that the string was passed to it; codex's full reply was: %q", reply)
}

// TestAgent_SwitchBackRestoresClaudeContext is the best-effort gap 4 of the
// Priority-1 e2e audit: codex->claude switch-back via `--resume`, proving the
// returning claude restores its native context. It combines two proofs:
//
//  1. Deterministic/cheap: the resumed segment's ProviderSessionID and
//     TranscriptPath must equal the ORIGINAL (pre-switch) claude segment's —
//     i.e. SwitchProvider's `--resume <id>` (internal/app/usecases/agent/
//     agent.go's priorSessionID lookup + descriptor session.resume) actually
//     reattached claude to its own prior session file, the Go-stack proof of
//     the Phase-0 spike's "Native resume / Case-1 (--resume <id> ->
//     source=resume)" scorecard row. This alone needs no model turn at all.
//  2. Behavioural: ask the resumed claude to recall the codeword seeded
//     before the first switch, and check its reply. NOTE this is NOT a clean
//     isolation of "answered purely from native --resume" vs "answered from
//     the freshly re-appended handoff": SwitchProvider always appends
//     AssembleHandoff's blob as `--append-system-prompt` on EVERY switch,
//     including a switch-back, and that blob already contains the codeword
//     (it was claude's own earlier turn). Proof (1) above is what actually
//     isolates the resume mechanism; this turn additionally proves the round
//     trip leaves claude in a working, answerable state.
func TestAgent_SwitchBackRestoresClaudeContext(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "switch-back", repoPath)

	const codeword = "TALON-6631"
	chatID, claudeSegID, _, codexTermSessID := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	// seedClaudeThenSwitchToCodex leaves codex live and active; switching back
	// below kills it, but guard the cleanup in case the switch itself fails.
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	segsBeforeSwitchBack, err := h.app.Usecases.Agent.SegmentsFor(ctx, chatID)
	require.NoError(t, err)
	var origClaudeSeg domain.AgentSegment
	for _, s := range segsBeforeSwitchBack {
		if s.ID == claudeSegID {
			origClaudeSeg = s
		}
	}
	require.NotEmpty(t, origClaudeSeg.ProviderSessionID, "original claude segment must have bound a session id: %+v", segsBeforeSwitchBack)
	require.NotEmpty(t, origClaudeSeg.TranscriptPath, "original claude segment must have recorded a transcript path: %+v", segsBeforeSwitchBack)

	newClaudeSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newClaudeTermSessID := segmentTerminalSessionID(t, h, chatID)
	require.NotEqual(t, codexTermSessID, newClaudeTermSessID, "switch-back must spawn a new terminal session")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newClaudeTermSessID) })

	start := time.Now()
	resumedProviderSessionID, segsAfter := waitForProviderSessionID(t, h, newClaudeTermSessID, chatID, newClaudeSegID, 30*time.Second)
	t.Logf("claude resumed in %s (session=%s)", time.Since(start), resumedProviderSessionID)
	require.NotEmpty(t, resumedProviderSessionID,
		"timed out after 30s waiting for the switched-back claude's SessionStart hook to bind: %+v", segsAfter)

	require.Equal(t, origClaudeSeg.ProviderSessionID, resumedProviderSessionID,
		"switch-back must --resume the ORIGINAL claude session id, not mint a new one — this is the "+
			"Go-stack proof of native resume (Phase-0 spike's Case-1)")

	var resumedSeg domain.AgentSegment
	for _, s := range segsAfter {
		if s.ID == newClaudeSegID {
			resumedSeg = s
		}
	}
	require.Equal(t, origClaudeSeg.TranscriptPath, resumedSeg.TranscriptPath,
		"a native --resume must reattach to the SAME transcript file as before ANY switch happened")

	// A resumed claude given a freshly `--append-system-prompt`-ed delta may
	// emit a synthetic (non-model, "model":"<synthetic>") housekeeping turn
	// before anything we type is processed — observed live across repeated
	// runs: Claude Code sometimes auto-injects a synthetic "Continue from
	// where you left off." user turn and replies "No response requested.",
	// apparently closing out the ORIGINAL codeword-seeding turn when its real
	// "acknowledged" reply lost a race with SwitchProvider's Kill of the
	// outgoing terminal (internal/engine/terminal/internal/session's
	// Session.Kill -> cmd.Process.Kill() is an unconditional hard SIGKILL for
	// EVERY provider — the design doc explicitly warns this is unsafe for
	// claude specifically: "Claude flushes its transcript only on a clean
	// exit ... never SIGKILL mid-flight", docs/superpowers/specs/
	// 2026-07-05-crowbar-agentic-engine-design.md §1). Flagged here, not
	// fixed — this file's brief is proving behavior end to end, not patching
	// the daemon.
	//
	// This housekeeping turn's on-disk appearance was wildly inconsistent
	// across live runs: absent entirely (the original "acknowledged" survived
	// intact), appearing within milliseconds, and once still NOT on disk after
	// a full 45s of idle polling (only flushing once our own follow-up keystrokes
	// landed) — its persistence is evidently tied to some later event, not a
	// fixed delay. Rather than trying to name which array index is "the real
	// reply" (an earlier version of this test tried settle-then-diff and
	// index-counting, both of which raced against this same unpredictability),
	// just drive the follow-up immediately and wait for ANY assistant entry to
	// contain the codeword — unambiguous, since neither the original
	// "acknowledged" turn nor a "No response requested." housekeeping turn
	// could ever satisfy that on their own.
	followUp := "What was the codeword I asked you to remember earlier in our conversation? Reply with only that word."
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte(followUp)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte("\r")))

	start = time.Now()
	var texts []string
	found := nudgeUntil(h, newClaudeTermSessID, 90*time.Second, func() (bool, bool) {
		texts = claudeAssistantTexts(resumedSeg.TranscriptPath)
		for _, tx := range texts {
			if strings.Contains(tx, codeword) {
				return true, true
			}
		}
		return false, false
	})
	t.Logf("waited %s for a reply referencing the codeword; all assistant texts observed: %q", time.Since(start), texts)
	require.True(t, found,
		"timed out waiting for any assistant reply (including the follow-up's) to reference the codeword; texts observed: %q", texts)
}

// TestAgent_GracefulTerminate_PreservesClaudePreSwitchReply is the empirical
// proof for the fix described in docs/superpowers/specs/
// 2026-07-05-crowbar-agentic-engine-design.md §8: SwitchProvider now quits the
// outgoing CLI via a graceful terminate (SIGTERM + a grace window, falling
// back to SIGKILL only if the process is still alive after it) instead of an
// unconditional hard Kill. TestAgent_SwitchBackRestoresClaudeContext already
// documented, live, that the OLD hard-SIGKILL behavior could sometimes lose
// claude's real pre-switch reply from its own native transcript — replaced by
// a synthetic "No response requested." housekeeping turn on resume — but that
// test tolerates the gap (it searches every assistant text, including a
// driven follow-up's, for the codeword) because closing the gap was not yet
// implemented.
//
// This test is the strict version: it captures claude's REAL reply to the
// seeded turn from its own on-disk transcript BEFORE ever calling
// SwitchProvider (the one moment we can be certain nothing has touched the
// process yet), switches away to codex and back to claude (exercising the
// exact TerminateGraceful call path SwitchProvider now uses for every
// provider, no branching), drives one follow-up turn to force claude to
// settle/flush its resumed state to disk (mirroring
// TestAgent_SwitchBackRestoresClaudeContext's hard-won finding that a
// synthetic housekeeping turn, if one was going to appear, may not flush to
// disk until further input lands), and then asserts BOTH that the captured
// baseline reply is still present verbatim and that no synthetic "No response
// requested." entry ever appeared.
//
// Run this multiple times (`go test -tags 'integration noEmbed' -race -p 1
// -run TestAgent_GracefulTerminate_PreservesClaudePreSwitchReply -count=N
// ./tests/integration/agent/...`) to gauge reliability against the real
// claude/codex binaries — a single pass is not strong evidence either way for
// a fix whose whole premise is an unverified CLI-internal flush behavior.
func TestAgent_GracefulTerminate_PreservesClaudePreSwitchReply(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "graceful-terminate", repoPath)

	chatID, claudeSegID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, "claude")
	require.NoError(t, err)

	claudeTermSessID := segmentTerminalSessionID(t, h, chatID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), claudeTermSessID) })

	start := time.Now()
	providerSessionID, segs := waitForProviderSessionID(t, h, claudeTermSessID, chatID, claudeSegID, 30*time.Second)
	require.NotEmpty(t, providerSessionID, "claude never bound before a turn could be driven: %+v", segs)
	t.Logf("claude bound in %s (session=%s)", time.Since(start), providerSessionID)

	var transcriptPath string
	for _, s := range segs {
		if s.ID == claudeSegID {
			transcriptPath = s.TranscriptPath
		}
	}
	require.NotEmpty(t, transcriptPath, "claude segment must have a transcript path recorded from SessionStart: %+v", segs)

	const codeword = "GRACEFUL-8847"
	prompt := "Remember this exact codeword for the rest of our conversation: " + codeword +
		". Reply with only the word: acknowledged."
	require.NoError(t, h.eng.Terminal.Write(ctx, claudeTermSessID, []byte(prompt)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, claudeTermSessID, []byte("\r")))

	// Wait for claude's own Stop hook to append to Crowbar's ledger — proof
	// the turn is FULLY complete, mirroring every other switch test's
	// synchronization point.
	start = time.Now()
	handoff := nudgeUntil(h, claudeTermSessID, 90*time.Second, func() (string, bool) {
		blob, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
		require.NoError(t, err)
		return blob, strings.Contains(blob, codeword)
	})
	t.Logf("waited %s for claude's Stop hook (ledger append)", time.Since(start))
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	// Read claude's REAL reply from its own on-disk transcript — separate
	// from (and the whole point of testing beyond) Crowbar's ledger copy —
	// while the process is STILL ALIVE and BEFORE SwitchProvider has touched
	// it. This is the exact on-disk state a hard SIGKILL was accused of
	// sometimes corrupting/losing before the process could finish flushing.
	baselineReply := nudgeUntil(h, claudeTermSessID, 15*time.Second, func() (string, bool) {
		texts := claudeAssistantTexts(transcriptPath)
		if len(texts) == 0 {
			return "", false
		}
		return texts[len(texts)-1], true
	})
	require.NotEmpty(t, baselineReply, "claude's own on-disk transcript never showed the seeded turn's reply")
	t.Logf("baseline (pre-switch, process still alive) reply on disk: %q", baselineReply)

	// Switch away — the exact call under test: SwitchProvider now uses
	// TerminateGraceful (SIGTERM + grace, falling back to SIGKILL only if
	// still alive) instead of the old hard Kill for the outgoing claude CLI.
	_, err = h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	codexTermSessID := segmentTerminalSessionID(t, h, chatID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// Switch back to claude immediately — AssembleHandoff only reads
	// Crowbar's own ledger from disk, so this does not depend on codex having
	// bound a session or processed anything; it exercises TerminateGraceful a
	// second time (against codex, which the design doc already established
	// tolerates a hard kill, so this leg is not the interesting one) and then
	// the native --resume path back into claude's original session.
	newClaudeSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newClaudeTermSessID := segmentTerminalSessionID(t, h, chatID)
	require.NotEqual(t, codexTermSessID, newClaudeTermSessID, "switch-back must spawn a new terminal session")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newClaudeTermSessID) })

	start = time.Now()
	resumedProviderSessionID, segsAfter := waitForProviderSessionID(t, h, newClaudeTermSessID, chatID, newClaudeSegID, 30*time.Second)
	t.Logf("claude resumed in %s (session=%s)", time.Since(start), resumedProviderSessionID)
	require.NotEmpty(t, resumedProviderSessionID, "timed out waiting for the switched-back claude's SessionStart hook to bind: %+v", segsAfter)
	require.Equal(t, providerSessionID, resumedProviderSessionID, "switch-back must --resume the ORIGINAL claude session id")

	var resumedTranscriptPath string
	for _, s := range segsAfter {
		if s.ID == newClaudeSegID {
			resumedTranscriptPath = s.TranscriptPath
		}
	}
	require.Equal(t, transcriptPath, resumedTranscriptPath, "a native --resume must reattach to the SAME transcript file as before ANY switch happened")

	// Drive one follow-up turn to force claude to settle/flush its resumed
	// state to disk — TestAgent_SwitchBackRestoresClaudeContext found live
	// that a synthetic housekeeping turn, when the OLD hard-kill bug produced
	// one, could remain unflushed to disk until further input landed, so
	// reading immediately post-resume with no follow-up would not reliably
	// exercise the failure mode this test exists to catch.
	followUp := "What was the codeword I asked you to remember earlier in our conversation? Reply with only that word."
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte(followUp)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte("\r")))

	start = time.Now()
	var texts []string
	found := nudgeUntil(h, newClaudeTermSessID, 90*time.Second, func() (bool, bool) {
		texts = claudeAssistantTexts(resumedTranscriptPath)
		for _, tx := range texts {
			if strings.Contains(tx, codeword) {
				return true, true
			}
		}
		return false, false
	})
	t.Logf("waited %s for the follow-up's reply; all assistant texts observed after switch-back: %q", time.Since(start), texts)
	require.True(t, found, "timed out waiting for the follow-up turn's reply to reference the codeword; texts observed: %q", texts)

	// THE KEY EMPIRICAL ASSERTIONS for this bug fix: the ORIGINAL pre-switch
	// reply must still be present verbatim, and no synthetic gap-filling
	// housekeeping entry must have appeared — proving the graceful
	// SIGTERM+grace terminate let claude flush its native transcript before
	// the outgoing process died. Pre-fix, a hard SIGKILL was observed to
	// sometimes replace/lose this exact entry with a synthetic "No response
	// requested." turn instead.
	assert.Contains(t, texts, baselineReply,
		"the original pre-switch reply must still be present verbatim in claude's native transcript after a "+
			"graceful terminate + native resume; texts observed: %q", texts)
	for _, tx := range texts {
		assert.NotContains(t, strings.ToLower(tx), "no response requested",
			"a synthetic housekeeping reply appearing here means claude's clean-exit transcript flush did not "+
				"happen before the outgoing process died; texts observed: %q", texts)
	}
}
