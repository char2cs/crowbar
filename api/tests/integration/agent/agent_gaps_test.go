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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
// tiny real turn, and asserts: the resulting ledger entry is non-empty (via
// AssembleHandoff); a codex-tagged ASSISTANT .turn entry specifically exists
// physically on disk — the actual proof that codex's turn_stop hook (not its
// UserPromptSubmit hook, which separately writes a codex-tagged USER turn for
// the driven prompt and alone could satisfy a role-blind check) appended a
// ledger entry; and the driven codeword round-trips onto some codex-tagged
// turn on disk (that UserPromptSubmit-recorded user turn, which echoes the
// prompt verbatim — codex was asked to reply with only "acknowledged", so its
// own assistant text never contains the codeword).
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

	// AssembleHandoff's rendered blob goes non-empty as soon as ANY ledger
	// turn exists for this chat — including the codex-tagged USER turn its
	// OWN UserPromptSubmit hook writes for the prompt we just drove — so
	// waiting on blob != "" alone is not a wait for the turn_stop hook at all:
	// observed live, it is satisfied in well under a second, long before
	// codex's real model reply and Stop hook actually run. Poll the ledger
	// directly for a codex-tagged ASSISTANT turn specifically — the actual
	// on-disk signal that turn_stop -> ledger.Append ran — and only then treat
	// the wait as done. An earlier version of this test waited on the blob
	// alone and read the ledger exactly once right after, which raced the
	// real Stop hook and would have passed even if turn_stop never appended
	// an assistant entry.
	start = time.Now()
	var turns []ledgerTurn
	nudgeUntil(h, termSessID, 90*time.Second, func() (bool, bool) {
		turns = readLedgerTurns(t, h, wsID, chatID)
		ok := len(assistantReplies(turns, "codex")) > 0
		return ok, ok
	})
	t.Logf("waited %s for codex's Stop hook (assistant ledger append); turns observed=%+v", time.Since(start), turns)
	require.NotEmpty(t, assistantReplies(turns, "codex"),
		"timed out waiting for codex's turn_stop hook to append an ASSISTANT .turn ledger entry after driving "+
			"a real codex turn; this proves codex's own Stop hook never reached /v0/agent/hooks, or "+
			"turn_stop -> ledger.Append never ran; turns observed: %+v", turns)

	handoff, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	// The doc comment's second half: the entry is PHYSICALLY ON DISK, a .turn
	// JSON record tagged with the codex provider id. The wait above already
	// proved a codex-tagged ASSISTANT turn exists — the actual proof this test
	// is named for, that codex's turn_stop hook (not its UserPromptSubmit
	// hook, which separately writes a codex-tagged USER turn carrying our
	// full prompt) appended a ledger entry. What remains is a content
	// round-trip check: the driven codeword must land on SOME codex-tagged
	// turn on disk (that UserPromptSubmit-recorded user turn, which echoes
	// the prompt verbatim — codex was asked to reply with only
	// "acknowledged", so its own assistant text never contains the codeword,
	// and this check is deliberately not scoped to the assistant turn).
	var codexTagged, carriesCodeword bool
	for _, tn := range turns {
		if tn.Provider != "codex" {
			continue
		}
		codexTagged = true
		if strings.Contains(tn.Text, codeword) {
			carriesCodeword = true
		}
	}
	require.True(t, codexTagged,
		"a .turn ledger entry must be physically on disk tagged with the codex provider id; turns=%+v", turns)
	require.True(t, carriesCodeword,
		"the codeword must round-trip onto some codex-tagged ledger turn on disk (the UserPromptSubmit-recorded "+
			"user turn, which echoes the driven prompt verbatim); turns=%+v", turns)
}

// ledgerTurn mirrors the on-disk JSON shape of one Crowbar ledger entry
// (internal/app/ledger.Turn): a single conversation turn Crowbar recorded from
// a vendor CLI's own UserPromptSubmit/Stop hook. Under descriptor-v2 this is
// Crowbar's OWN hook-derived record — the oracle for "what each side said" —
// NOT a vendor transcript. v2 reads no vendor transcript and records no
// transcript path, so the tests below assert on this ledger instead.
type ledgerTurn struct {
	Role     string `json:"role"`     // "user" | "assistant"
	Provider string `json:"provider"` // the segment provider that produced the turn
	Text     string `json:"text"`
}

// readLedgerTurns reads chatID's ledger directory — the same
// <workspaceRoot>/chats/<chatID>/ledger path worktreepath.AgentLedgerDir
// resolves and AssembleHandoff renders from, where workspaceRoot is the parent
// of the workspace's git worktree (the workspace-root split, spec §3.5) — and
// returns every recorded turn in chronological order. The worktree path is
// resolved from the live workspace read model (h.app.Repositories.Workspace)
// rather than reconstructed from ids, so the ledger dir always tracks wherever
// the workspace was actually provisioned. AppendTurn names its entries with an
// %08d sequence prefix, so the .turn filenames sort lexically == chronologically.
// Returns nil when the ledger has no entries yet. worktreepath is doubly-internal
// (under app/usecases/internal) and not importable from this test package, so the
// ledger dir is built inline with the same shape as worktreepath.AgentLedgerDir.
func readLedgerTurns(t *testing.T, h *harness, wsID, chatID string) []ledgerTurn {
	t.Helper()
	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err, "resolve workspace %s for its worktree path", wsID)
	require.NotEmpty(t, ws.WorktreePath, "workspace %s has no worktree path", wsID)
	// filepath.Dir(worktree) == workspaceRoot; ledger lives at
	// <workspaceRoot>/chats/<chatID>/ledger (== worktreepath.AgentLedgerDir).
	dir := filepath.Join(filepath.Dir(ws.WorktreePath), "chats", chatID, "ledger")
	des, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err, "read ledger dir %s", dir)

	var names []string
	for _, de := range des {
		if !de.IsDir() && strings.HasSuffix(de.Name(), ".turn") {
			names = append(names, de.Name())
		}
	}
	sort.Strings(names)

	var turns []ledgerTurn
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		require.NoError(t, err, "read ledger entry %s", n)
		var tn ledgerTurn
		require.NoError(t, json.Unmarshal(data, &tn), "unmarshal ledger entry %s", n)
		turns = append(turns, tn)
	}
	return turns
}

// assistantReplies returns, in order, the text of every ASSISTANT turn the
// given provider produced — the model's OWN Stop-hook output, isolated from
// echoed user prompts and handoff text. This is the ledger analogue of the old
// per-vendor transcript readers, but it reads Crowbar's own record and can
// attribute a reply to the exact provider that generated it — which the
// rendered AssembleHandoff blob (every turn flattened into one string) cannot.
func assistantReplies(turns []ledgerTurn, provider string) []string {
	var out []string
	for _, tn := range turns {
		if tn.Role == "assistant" && tn.Provider == provider {
			out = append(out, tn.Text)
		}
	}
	return out
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
			// vacated-chat clear in moveToNewChat — if /clear yields more than one
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
	require.Equal(t, "ended", originalSegs[0].Status, "the original segment must be marked ended, not left dangling as active")
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

	codexTermSessID = waitForSegmentTerminalSessionID(t, h, chatID, newSegID, 5*time.Second)
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
// asserts CODEX'S OWN REPLY (isolated from the echoed user/handoff turns by
// reading only assistant turns tagged with the codex provider in Crowbar's
// ledger — codex's own Stop-hook output) contains it — the Go-stack proof of
// the Phase-0 spike's "Codex read Claude's raw 38 KB .jsonl and extracted the
// codeword" finding (docs/superpowers/specs/
// 2026-07-05-crowbar-agentic-engine-design.md §1 scorecard).
//
// The ledger — not a vendor transcript — is the v2 oracle. AssembleHandoff's
// rendered blob is NOT usable for this check: it flattens EVERY turn (claude's
// seeding turn, the echoed handoff, the follow-up prompt) into one string that
// already contains the codeword, so "handoff contains codeword" would be
// trivially true without proving codex generated anything. Reading only the
// codex-tagged ASSISTANT turns isolates codex's own model output.
func TestAgent_CodexUsesHandoff(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-handoff", repoPath)

	const codeword = "OSPREY-4482"
	chatID, _, _, codexTermSessID := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// codex's first turn is the handoff itself (auto-submitted as its initial
	// positional prompt once past the trust dialog — see
	// seedClaudeThenSwitchToCodex's doc comment). It carries no instruction of
	// its own (AssembleHandoff just wraps the raw ledger in a header/footer), so
	// codex free-associates a short reply to it (observed live: a bare
	// "acknowledged", apparently echoing the pattern in claude's handed-off
	// transcript) — wait for that first turn's assistant reply to land in the
	// ledger BEFORE typing our own follow-up, so a race between "our follow-up's
	// reply" and "the auto-turn's own reply" can't produce a false pass (this
	// raced and DID false-pass on the first version of this test: baseline was
	// captured before turn 1 had actually finished, and the wait below matched
	// turn 1's own "acknowledged" instead of our follow-up). We then require a
	// NEW codex assistant turn (index beyond the baseline count) to carry the
	// codeword, so even a codex auto-reply that happened to echo it could not
	// satisfy the assertion — only a reply codex produced AFTER we asked can.
	baselineStart := time.Now()
	var baselineCount int
	nudgeUntil(h, codexTermSessID, 30*time.Second, func() (bool, bool) {
		baselineCount = len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex"))
		return true, baselineCount >= 1
	})
	t.Logf("waited %s for codex's auto-submitted-handoff turn to land in the ledger (%d codex assistant turns)",
		time.Since(baselineStart), baselineCount)
	require.GreaterOrEqual(t, baselineCount, 1,
		"timed out waiting for codex's first (auto-submitted handoff) turn to append an assistant .turn entry")

	followUp := "What exact codeword appeared in the context you were given above? Reply with only that word."
	require.NoError(t, h.eng.Terminal.Write(ctx, codexTermSessID, []byte(followUp)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, codexTermSessID, []byte("\r")))

	start := time.Now()
	reply := nudgeUntil(h, codexTermSessID, 90*time.Second, func() (string, bool) {
		replies := assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex")
		for _, r := range replies[min(baselineCount, len(replies)):] {
			if strings.Contains(r, codeword) {
				return r, true
			}
		}
		return "", false
	})
	t.Logf("waited %s for codex's reply to the follow-up turn", time.Since(start))
	require.NotEmpty(t, reply, "timed out waiting for codex to produce a NEW assistant reply (after the auto-handoff turn) referencing the codeword")
	require.Contains(t, reply, codeword,
		"codex's own reply must reference the codeword handed off from claude's session — proves codex "+
			"actually READ the opaque handoff, not just that the string was passed to it; codex's full reply was: %q", reply)
}

// TestAgent_SwitchBackRestoresClaudeContext is the best-effort gap 4 of the
// Priority-1 e2e audit: codex->claude switch-back via `--resume`, proving the
// returning claude restores its native context. It combines two proofs:
//
//  1. Deterministic/cheap: the resumed segment's ProviderSessionID must equal
//     the ORIGINAL (pre-switch) claude segment's — i.e. SwitchProvider's
//     `--resume <id>` (internal/app/usecases/agent/agent.go's priorSessionID
//     lookup + descriptor session.resume) actually reattached claude to its own
//     prior native session, the Go-stack proof of the Phase-0 spike's "Native
//     resume / Case-1 (--resume <id> -> source=resume)" scorecard row. Under
//     descriptor-v2 the session id is the continuity witness (Crowbar no longer
//     records a vendor transcript path). This alone needs no model turn at all.
//  2. Behavioural: ask the resumed claude to recall the codeword seeded
//     before the first switch, and check its reply — read from claude's own
//     Stop-hook assistant turns in Crowbar's ledger. NOTE this is NOT a clean
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

	newClaudeSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newClaudeTermSessID := waitForSegmentTerminalSessionID(t, h, chatID, newClaudeSegID, 5*time.Second)
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
	require.Equal(t, origClaudeSeg.ProviderSessionID, resumedSeg.ProviderSessionID,
		"native --resume reattaches the same provider session: the resumed segment persists the ORIGINAL "+
			"claude session id, not a freshly minted one (v2's continuity witness, replacing the pre-v2 "+
			"transcript-path equality)")

	// A resumed claude given a freshly `--append-system-prompt`-ed delta may
	// emit a synthetic "Continue from where you left off." / "No response
	// requested." housekeeping turn before anything we type is processed
	// (observed live across repeated runs pre-graceful-terminate). Whether such
	// a turn even reaches Crowbar's ledger depends on whether it fires claude's
	// Stop hook, which was inconsistent live. Rather than try to name which
	// assistant turn is "the real reply" (an earlier version raced on
	// index-counting), drive the follow-up immediately and wait for ANY of
	// claude's own assistant turns in the ledger to contain the codeword —
	// unambiguous, since neither the original "acknowledged" turn nor a "No
	// response requested." housekeeping turn could ever satisfy that on their
	// own. Reading claude-tagged assistant turns from Crowbar's ledger replaces
	// the pre-v2 read of claude's native transcript (which v2 no longer records
	// a path to).
	followUp := "What was the codeword I asked you to remember earlier in our conversation? Reply with only that word."
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte(followUp)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte("\r")))

	start = time.Now()
	var texts []string
	found := nudgeUntil(h, newClaudeTermSessID, 90*time.Second, func() (bool, bool) {
		texts = assistantReplies(readLedgerTurns(t, h, wsID, chatID), "claude")
		for _, tx := range texts {
			if strings.Contains(tx, codeword) {
				return true, true
			}
		}
		return false, false
	})
	t.Logf("waited %s for a reply referencing the codeword; all claude assistant turns observed: %q", time.Since(start), texts)
	require.True(t, found,
		"timed out waiting for any claude assistant turn (including the follow-up's) to reference the codeword; turns observed: %q", texts)
}

// TestAgent_SwitchRoundTrip_ResumesAndAvoidsSyntheticLedgerTurn drives a full
// switch-away-then-switch-back round trip (claude -> codex -> claude) through
// SwitchProvider's TerminateGraceful call path (spec §8: SIGTERM + a grace
// window, falling back to SIGKILL only if the process is still alive after
// it, applied uniformly to every provider, no per-provider branching) and
// verifies two things once claude comes back:
//
//  1. Native resume continuity: the resumed segment's ProviderSessionID
//     equals the ORIGINAL (pre-switch) claude segment's — i.e. SwitchProvider's
//     `--resume <id>` (agent.go's priorSessionID lookup + descriptor
//     session.resume) actually reattached claude to its own prior native
//     session rather than minting a new one.
//  2. No synthetic housekeeping turn: after driving a follow-up turn to force
//     claude to settle its resumed state, none of claude's own assistant
//     turns recorded in Crowbar's ledger contain a synthetic "No response
//     requested." housekeeping reply — the kind of gap-filling turn a resume
//     over a corrupted native session can produce.
//
// v2 CAVEAT (honest scope): this test does NOT verify vendor-native-transcript
// flush integrity, i.e. it cannot distinguish "the outgoing claude CLI exited
// cleanly via SIGTERM and flushed its own transcript" from "claude was
// SIGKILLed mid-flight". Pre-v2 that distinction was directly observable by
// reading claude's native transcript file, where a hard kill could corrupt or
// replace the last written reply. Under descriptor-v2 Crowbar never reads the
// vendor transcript at all — the ledger is built entirely from claude's own
// turn_stop/user_prompt hooks, and the hook that recorded the pre-switch reply
// runs to completion and appends to the append-only ledger BEFORE
// SwitchProvider ever touches the process, so the ledger's copy of that reply
// is durable no matter how the outgoing process later dies. That distinction
// is simply no longer observable to Crowbar, so this test does not assert on
// it. The no-synthetic-turn check above is the one signal that still tracks
// graceful vs. hard termination, and even it is best-effort: whether a
// synthetic housekeeping turn (if the CLI ever emits one) fires claude's Stop
// hook at all was observed to be inconsistent live.
//
// Run this multiple times (`go test -tags 'integration noEmbed' -race -p 1
// -run TestAgent_SwitchRoundTrip_ResumesAndAvoidsSyntheticLedgerTurn -count=N
// ./tests/integration/agent/...`) to gauge reliability against the real
// claude/codex binaries — a single pass is not strong evidence either way for
// a fix whose whole premise is an unverified CLI-internal flush behavior.
func TestAgent_SwitchRoundTrip_ResumesAndAvoidsSyntheticLedgerTurn(t *testing.T) {
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

	// Switch away — the exact call under test: SwitchProvider now uses
	// TerminateGraceful (SIGTERM + grace, falling back to SIGKILL only if
	// still alive) instead of the old hard Kill for the outgoing claude CLI.
	codexSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	codexTermSessID := waitForSegmentTerminalSessionID(t, h, chatID, codexSegID, 5*time.Second)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// Switch back to claude immediately — AssembleHandoff only reads
	// Crowbar's own ledger from disk, so this does not depend on codex having
	// bound a session or processed anything; it exercises TerminateGraceful a
	// second time (against codex, which the design doc already established
	// tolerates a hard kill, so this leg is not the interesting one) and then
	// the native --resume path back into claude's original session.
	newClaudeSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newClaudeTermSessID := waitForSegmentTerminalSessionID(t, h, chatID, newClaudeSegID, 5*time.Second)
	require.NotEqual(t, codexTermSessID, newClaudeTermSessID, "switch-back must spawn a new terminal session")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newClaudeTermSessID) })

	start = time.Now()
	resumedProviderSessionID, segsAfter := waitForProviderSessionID(t, h, newClaudeTermSessID, chatID, newClaudeSegID, 30*time.Second)
	t.Logf("claude resumed in %s (session=%s)", time.Since(start), resumedProviderSessionID)
	require.NotEmpty(t, resumedProviderSessionID, "timed out waiting for the switched-back claude's SessionStart hook to bind: %+v", segsAfter)
	require.Equal(t, providerSessionID, resumedProviderSessionID,
		"switch-back must --resume the ORIGINAL claude session id (v2's native-resume continuity witness, "+
			"replacing the pre-v2 transcript-path equality)")

	// Drive one follow-up turn to force claude to settle its resumed state —
	// TestAgent_SwitchBackRestoresClaudeContext found live that a synthetic
	// housekeeping turn, when the OLD hard-kill bug produced one, could remain
	// unsurfaced until further input landed, so reading immediately post-resume
	// with no follow-up would not reliably exercise the failure mode this test
	// exists to catch.
	followUp := "What was the codeword I asked you to remember earlier in our conversation? Reply with only that word."
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte(followUp)))
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, newClaudeTermSessID, []byte("\r")))

	start = time.Now()
	var texts []string
	found := nudgeUntil(h, newClaudeTermSessID, 90*time.Second, func() (bool, bool) {
		texts = assistantReplies(readLedgerTurns(t, h, wsID, chatID), "claude")
		for _, tx := range texts {
			if strings.Contains(tx, codeword) {
				return true, true
			}
		}
		return false, false
	})
	t.Logf("waited %s for the follow-up's reply; all claude assistant turns observed after switch-back: %q", time.Since(start), texts)
	require.True(t, found, "timed out waiting for the follow-up turn's reply to reference the codeword; turns observed: %q", texts)

	// THE KEY ASSERTION for this round trip (see the v2 CAVEAT above for its
	// best-effort strength): no synthetic gap-filling "No response requested."
	// housekeeping entry must have reached claude's own assistant turns — the
	// surviving regression signal that the graceful SIGTERM+grace terminate
	// let claude exit cleanly instead of being SIGKILLed mid-flight.
	for _, tx := range texts {
		assert.NotContains(t, strings.ToLower(tx), "no response requested",
			"a synthetic housekeeping reply appearing here means claude did not exit cleanly before the "+
				"outgoing process died; turns observed: %q", texts)
	}
}
