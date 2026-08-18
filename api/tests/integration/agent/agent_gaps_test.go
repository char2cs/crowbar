//go:build integration

// This file closes the remaining Priority-1 end-to-end gaps identified after
// the initial acceptance pass (agent_test.go's TestAgent_ClaudeSpawnAndDetect /
// TestAgent_SwitchClaudeToCodex): a real codex turn_stop -> ledger round trip,
// the reducer's "registered" outcome (a live /clear) through the real Go
// stack, and the receiving provider actually USING a cross-provider handoff
// (not just carrying the string). Reuses newHarness/importRepoAndWorkspace/
// requireCLI/liveRunnerTerminalSession/waitForProviderSessionID/nudgeUntil from
// agent_test.go.
package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

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

	// A bare codex spawn paints an interactive trust dialog — a Crowbar-managed
	// child workspace is a git worktree, which codex's trust check treats as "a
	// subdirectory of a Git project" — and emits NO hook traffic behind it. So the
	// dialog is what has to be waited for FIRST, and it is a real, observable thing
	// to wait for: spawnReady blocks until codex has painted it, then dismisses it
	// with one Enter.
	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "codex")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned codex: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	const codeword = "MERIDIAN-2260"
	// Text and the submitting Enter are separate writes (a trailing \r in the same
	// write as pasted text lands as a literal newline inside the input box, not a
	// submit — codex's TUI is Ink-style too), and drive blocks on codex ECHOING the
	// text into its composer in between. That echo is also what proves codex has
	// finished booting its MCP servers and is accepting input at all: if the text
	// had landed in a splash screen it would never come back.
	drive(t, h, tap, termSessID, "Remember this exact codeword for the rest of our conversation: "+codeword+
		". Reply with only the word: acknowledged.")

	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID,
		"codex's SessionStart hook never reached /v0/agent/hooks to bind a conversation; this means either "+
			"codex never started in the PTY, its SessionStart hook never fired, `crowbar hook` could not reach "+
			"the unix socket, or IngestHook/the reducer did not persist the outcome — runner observed: %+v", runner)

	// AssembleHandoff's rendered blob goes non-empty as soon as ANY ledger turn
	// exists for this chat — including the codex-tagged USER turn its OWN
	// UserPromptSubmit hook writes for the prompt we just drove — so waiting on
	// blob != "" is not a wait for the turn_stop hook at all: it is satisfied in
	// well under a second, long before codex's real model reply and Stop hook run.
	// Wait for a codex-tagged ASSISTANT turn specifically — the actual recorded
	// signal that turn_stop -> AppendTurn ran. An earlier version of this test
	// waited on the blob alone and read the record once right after, which raced
	// the real Stop hook and would have passed even if turn_stop never appended an
	// assistant entry.
	awaitHook(t, h, "codex to append an ASSISTANT ledger turn", func() (bool, bool) {
		ok := len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex")) > 0
		return ok, ok
	})
	turns := readLedgerTurns(t, h, wsID, chatID)
	require.NotEmpty(t, assistantReplies(turns, "codex"),
		"codex's turn_stop hook never appended an ASSISTANT turn after a real codex turn; this "+
			"proves codex's own Stop hook never reached /v0/agent/hooks, or turn_stop -> AppendTurn never "+
			"ran; turns observed: %+v", turns)

	handoff, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	// The doc comment's second half: the entry is DURABLY RECORDED, tagged with
	// the codex provider id. The wait above already proved a codex-tagged
	// ASSISTANT turn exists — the actual proof this test is named for, that
	// codex's turn_stop hook (not its UserPromptSubmit hook, which separately
	// writes a codex-tagged USER turn carrying our full prompt) appended a turn.
	// What remains is a content round-trip check: the driven codeword must land
	// on SOME codex-tagged turn in the record (that UserPromptSubmit-recorded
	// user turn, which echoes the prompt verbatim — codex was asked to reply with
	// only "acknowledged", so its own assistant text never contains the codeword,
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
		"a turn must be durably recorded tagged with the codex provider id; turns=%+v", turns)
	require.True(t, carriesCodeword,
		"the codeword must round-trip onto some codex-tagged turn in the record (the UserPromptSubmit-recorded "+
			"user turn, which echoes the driven prompt verbatim); turns=%+v", turns)
}

// readLedgerTurns returns chatID's whole conversation in order: a single turn
// Crowbar recorded from a vendor CLI's own UserPromptSubmit/Stop hook. Under
// descriptor-v2 this is Crowbar's OWN hook-derived record — the oracle for "what
// each side said" — NOT a vendor transcript. v2 reads no vendor transcript and
// records no transcript path, so the tests below assert on this record instead.
//
// It reads through Agent.ReadMessages, the same reader the Chat pane's message
// endpoint serves from. It used to read the flat-file ledger directory at
// <workspaceRoot>/chats/<chatID>/ledger; that store was replaced by the
// agentactivity aggregate and NOTHING writes those files any more, so the old
// body silently returned nil for every chat — which made every assertion built on
// it either vacuously empty or a five-minute backstop expiry. The lesson is the
// same one that broke this package's compile: nothing behind the integration tag
// is exercised by a default `go test ./...`.
//
// wsID is retained because resolving the workspace is a real precondition: a chat
// whose workspace has gone is a fixture fault, and failing on it here says so
// rather than surfacing as an empty conversation.
//
// Returns nil when the chat has no turns yet.
func readLedgerTurns(t *testing.T, h *harness, wsID, chatID string) []ledgerTurn {
	t.Helper()
	ctx := context.Background()
	_, err := h.app.Repositories.Workspace.Get(ctx, wsID)
	require.NoError(t, err, "resolve workspace %s the chat is expected on", wsID)

	// after=0/before=0 is the newest window; maxMessagePageLimit is 200, and no
	// fixture in this package drives anything near that many turns.
	page, err := h.app.Usecases.Agent.ReadMessages(ctx, chatID, 0, 0, 200)
	require.NoError(t, err, "read chat %s's conversation record", chatID)

	var turns []ledgerTurn
	for _, m := range page.Items {
		turns = append(turns, ledgerTurn{Role: m.Role, Provider: m.Provider, Text: m.Text})
	}
	return turns
}

// ledgerTurn is the slice of a recorded turn these tests assert on: who spoke,
// which provider produced it, and what was said.
type ledgerTurn struct {
	Role     string // "user" | "assistant"
	Provider string // the provider of the RUNNER that produced the turn
	Text     string
}

// assistantReplies returns, in order, the text of every ASSISTANT turn the
// given provider produced — the model's OWN Stop-hook output, isolated from
// echoed user prompts and handoff text. This is the record analogue of the old
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
// daemon reacted correctly end to end: the RUNNER moved to a brand-new AgentChat,
// carrying a DIFFERENT native session id but the SAME runner id and the SAME PTY
// (one process, which merely started a new conversation), and the chat it left is
// dormant but intact.
//
// The PTY assertion is the one that matters most to a user: /clear must not remount
// their terminal. Under the old segment model it did — the pane was attached to a
// terminal session named by a segment row that /clear ended, so the pane had to find
// the new segment and re-attach, taking the scrollback with it. Nothing about the
// process ever changed.
func TestAgent_LiveClearRegistersNewChat(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-clear", repoPath)

	originalChatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")

	originalProviderSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, originalProviderSessionID, "claude never bound before /clear could be driven: %+v", runner)

	// Drive a real /clear, blocking on claude echoing it into the composer before
	// the submitting Enter (the two writes cannot be merged: a trailing \r in the
	// same write lands as a literal newline, not a submit).
	drive(t, h, tap, termSessID, "/clear")

	// Watch the RUNNER, not the chat list. Its id is stable across the move — that is
	// the whole point of the model — so "has the CLI moved yet?" is a single-row read of
	// the process itself. The old version had to scan every chat for a non-original one
	// that already carried an ActiveSegmentID, precisely because a chat could be
	// observed half-formed (or already vacated again) mid-registration; there is no such
	// state to trip over when the thing you ask is the process.
	//
	// The move is announced by /clear re-firing claude's SessionStart with a CHANGED
	// id, so the arriving hook is exactly the right thing to block on.
	moved := awaitHook(t, h, "/clear to move the runner into a new chat", func() (domain.AgentRunner, bool) {
		r, err := h.app.Repositories.AgentRunner.Get(ctx, runnerID)
		if err != nil {
			return domain.AgentRunner{}, false // its CLI died
		}
		return r, r.CurrentChatID != "" && r.CurrentChatID != originalChatID
	})

	newChatID := moved.CurrentChatID
	require.NotEqual(t, originalChatID, newChatID,
		"the runner never moved to a NEW chat after driving a real /clear; this means either /clear never "+
			"reached claude's TUI, SessionStart did not re-fire with a changed id, or the reducer did not "+
			"persist a \"move to a new chat\" outcome — runner: %+v", moved)
	require.NotEmpty(t, newChatID, "a moved runner is placed on a real chat, never nowhere")

	// The chat really exists, and the SAME process is on it.
	newChat, err := h.app.Usecases.Agent.GetChat(ctx, newChatID)
	require.NoError(t, err)
	require.Equal(t, newChatID, newChat.ID, "the chat /clear minted must be readable by id")

	require.Equal(t, runnerID, liveRunnerID(t, h, newChatID),
		"the new chat's live runner must be the SAME runner — /clear moves the process, it does not spawn one")
	require.Equal(t, termSessID, moved.TerminalSession,
		"...and it brings its PTY with it. The terminal session must be IDENTICAL across a /clear: this is "+
			"what lets the chat pane follow a conversation switch without remounting the user's terminal")

	require.NotEmpty(t, moved.CurrentSession, "the moved runner must carry the newly minted native session id")
	require.NotEqual(t, originalProviderSessionID, moved.CurrentSession,
		"/clear must mint a DIFFERENT native session id — that changed id is the ONLY signal Crowbar has")
	require.Equal(t, []string{moved.CurrentSession}, chatSessionIDs(t, h, newChatID),
		"the new chat hosts exactly the one conversation that minted it")

	// The chat it left is DORMANT — nothing points at it — but INTACT.
	require.Empty(t, liveRunnerID(t, h, originalChatID),
		"the vacated chat must have no live runner: the CLI walked out of it, and a chat still advertising "+
			"a runner that has left would hand a pane the PTY of a conversation the user just cleared")
	require.Equal(t, []string{originalProviderSessionID}, chatSessionIDs(t, h, originalChatID),
		"the vacated chat keeps its own conversation history — append-only, describing no process, and the "+
			"only thing that makes it revivable")
}

// seedClaudeThenSwitchToCodex spawns claude, seeds it with codeword via one
// real turn, waits for the resulting ledger entry, switches the chat to
// codex, and brings the incoming codex up to a drivable state — exactly the
// sequence TestAgent_SwitchClaudeToCodex proves, factored out so the gap 3/4
// tests below don't each re-derive ~30 lines of setup. Returns the chat id,
// the claude runner id and the conversation it bound, the new codex runner id,
// the codex runner's terminal session id, and a tap on its PTY.
//
// The incoming codex needs its trust dialog dismissed exactly as a bare spawn
// does. It did not used to: the handoff was injected as codex's POSITIONAL
// prompt, so it sat pre-loaded in the input box and a stray nudge Enter submitted
// it as codex's opening turn. That was the bug — codex answered Crowbar's handoff
// on sight instead of waiting for the user. A switched-to codex now receives the
// handoff through the silent `developer_instructions` config channel (codex.yaml's
// context_inject) and comes up IDLE with an empty composer.
func seedClaudeThenSwitchToCodex(
	t *testing.T,
	h *harness,
	wsID string,
	codeword string,
) (chatID, claudeRunnerID, claudeSessionID, codexRunnerID, codexTermSessID string, codexTap *kit.PTYTap) {
	t.Helper()
	ctx := context.Background()

	chatID, claudeRunnerID, claudeTermSessID, claudeTap := spawnReady(t, h, wsID, "claude")

	claudeSessionID, runner := awaitSessionBound(t, h, claudeRunnerID, claudeTermSessID, claudeTap)
	require.NotEmpty(t, claudeSessionID, "claude never bound a session before a turn could be driven: %+v", runner)

	drive(t, h, claudeTap, claudeTermSessID, "Remember this exact codeword for the rest of our conversation: "+
		codeword+". Reply with only the word: acknowledged.")

	// claude must FINISH its turn before it is switched away from: the codeword is in
	// the ledger the moment the prompt is submitted, so that alone would switch mid-answer.
	awaitTurnComplete(t, h, wsID, chatID, "claude")
	handoff := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	codexRunnerID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	codexTermSessID = runnerTerminalSession(t, h, codexRunnerID)
	require.NotEqual(t, claudeTermSessID, codexTermSessID, "a switch must spawn a NEW PTY for the incoming codex CLI")

	// Bring codex past its trust dialog — and deliberately do NOT wait for it to bind
	// a conversation. It cannot be waited for: codex creates its rollout, and
	// therefore fires SessionStart, LAZILY on its first real turn, and a switched-to
	// codex is by design IDLE — the handoff reached it through the silent
	// developer_instructions channel, so it has an empty composer and nothing to
	// answer. Waiting for a session id here would be waiting on codex's own laziness
	// and would never return. Callers that need codex's conversation id must drive a
	// turn FIRST and await the bind after (TestAgent_CodexTurnAppendsLedger is the
	// model).
	codexTap = attachReady(t, h, codexTermSessID, "codex", codexRunnerID)

	// The switched-to codex must be IDLE, not answering the handoff: the document
	// went in through developer_instructions, which is not a user message, so codex
	// has nothing to reply to and the ledger has no codex turn.
	//
	// This is asserted at the one moment it is meaningful and provable: codex has just
	// told us, by painting its composer prompt, that it is up and waiting for input —
	// so "no codex turn in the ledger" is codex genuinely declining to answer the
	// handoff, not merely us looking too early. (The old code reached for the same
	// teeth by firing Enters at it for 30 seconds; being able to observe readiness
	// directly is strictly stronger, and does not type into the user's composer.)
	require.Empty(t, assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex"),
		"a switched-to codex must not answer the handoff on sight — it is context, not a prompt")

	return chatID, claudeRunnerID, claudeSessionID, codexRunnerID, codexTermSessID, codexTap
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

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-handoff", repoPath)

	const codeword = "OSPREY-4482"
	chatID, _, _, _, codexTermSessID, codexTap := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// seedClaudeThenSwitchToCodex has already asserted the switched-to codex is
	// IDLE — it has produced no turn, because the handoff reached it through
	// developer_instructions rather than as its opening user prompt. So codex's
	// FIRST assistant turn is necessarily a reply to what WE type, and it can
	// only carry the codeword if codex actually read the injected context. That
	// makes the old baseline-counting dance (needed when codex auto-answered the
	// handoff, and which once false-passed by matching the auto-turn's own reply)
	// unnecessary: there is nothing to race against.
	drive(t, h, codexTap, codexTermSessID,
		"What exact codeword appeared in the context you were given? Reply with only that word.")

	replies := awaitAssistantReply(t, h, wsID, chatID, "codex", codeword)
	require.NotEmpty(t, replies, "codex never replied referencing the codeword")
	require.Contains(t, strings.Join(replies, "\n"), codeword,
		"codex's own reply must reference the codeword handed off from claude's session — proves codex "+
			"actually READ the handoff injected via developer_instructions, not just that the string was "+
			"passed to it; codex's replies were: %q", replies)

	// The injected handoff must never be recorded as something the USER said: that
	// is what made each handoff nest inside the next one.
	for _, turn := range readLedgerTurns(t, h, wsID, chatID) {
		if turn.Role == "user" {
			require.NotContains(t, turn.Text, "HANDED-OFF CONTEXT",
				"Crowbar's own handoff document must never be recorded as a user turn")
		}
	}
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
//     Stop-hook assistant turns in Crowbar's ledger. This IS now a clean
//     isolation of "answered purely from native --resume". It used not to be:
//     SwitchProvider re-appended the WHOLE ledger on every switch, including a
//     switch-back, so the codeword was handed straight back to claude in its
//     system prompt and the reply proved nothing. A provider resumed into its
//     own session is now handed only the GAP — what happened while it was away —
//     and nothing happened here (codex bound a session and was switched away
//     from without taking a turn), so the gap is EMPTY: claude is spawned with
//     no conversation document at all. The only place the codeword can come from
//     is claude's own resumed session.
func TestAgent_SwitchBackRestoresClaudeContext(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "switch-back", repoPath)

	const codeword = "TALON-6631"
	chatID, _, origClaudeSessionID, _, codexTermSessID, _ := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	// seedClaudeThenSwitchToCodex leaves codex live and active; switching back
	// below kills it, but guard the cleanup in case the switch itself fails.
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// claude's original conversation is now part of the CHAT's append-only history —
	// it outlives the runner that opened it, which is exactly what a resume needs.
	require.NotEmpty(t, origClaudeSessionID, "the original claude runner must have bound a session id")
	require.Contains(t, chatSessionIDs(t, h, chatID), origClaudeSessionID,
		"the chat's conversation history must still carry claude's conversation after it was switched away from")

	newClaudeRunnerID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newClaudeTermSessID := runnerTerminalSession(t, h, newClaudeRunnerID)
	require.NotEqual(t, codexTermSessID, newClaudeTermSessID, "switch-back must spawn a new terminal session")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newClaudeTermSessID) })

	// The resumed claude is a FRESH PROCESS in a fresh PTY, so it shows its trust
	// dialog again and — exactly as at first spawn — fires no SessionStart until it
	// is dismissed. Blocking on that dialog is what makes the bind below arrive.
	newClaudeTap := attachReady(t, h, newClaudeTermSessID, "claude", newClaudeRunnerID)

	resumedProviderSessionID, resumedRunner := awaitSessionBound(t, h, newClaudeRunnerID, newClaudeTermSessID, newClaudeTap)
	require.NotEmpty(t, resumedProviderSessionID,
		"the switched-back claude's SessionStart hook never bound: %+v", resumedRunner)

	require.Equal(t, origClaudeSessionID, resumedProviderSessionID,
		"switch-back must --resume the ORIGINAL claude session id, not mint a new one — this is the "+
			"Go-stack proof of native resume (Phase-0 spike's Case-1)")
	require.Equal(t, origClaudeSessionID, resumedRunner.CurrentSession,
		"native --resume reattaches the same provider session: the resumed RUNNER holds the ORIGINAL claude "+
			"conversation id, not a freshly minted one (v2's continuity witness, replacing the pre-v2 "+
			"transcript-path equality)")
	require.Equal(t, newClaudeRunnerID, liveRunnerID(t, h, chatID),
		"the resumed claude CLI must be the one placed on the chat")

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
	drive(t, h, newClaudeTap, newClaudeTermSessID,
		"What was the codeword I asked you to remember earlier in our conversation? Reply with only that word.")

	texts := awaitAssistantReply(t, h, wsID, chatID, "claude", codeword)
	require.Contains(t, strings.Join(texts, "\n"), codeword,
		"no claude assistant turn (including the follow-up's) referenced the codeword; turns observed: %q", texts)
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

	chatID, claudeSegID, claudeTermSessID, claudeTap := spawnReady(t, h, wsID, "claude")

	providerSessionID, runner := awaitSessionBound(t, h, claudeSegID, claudeTermSessID, claudeTap)
	require.NotEmpty(t, providerSessionID, "claude never bound before a turn could be driven: %+v", runner)

	const codeword = "GRACEFUL-8847"
	drive(t, h, claudeTap, claudeTermSessID, "Remember this exact codeword for the rest of our conversation: "+
		codeword+". Reply with only the word: acknowledged.")

	// Block on claude's own Stop hook appending to Crowbar's ledger — proof the turn
	// is FULLY complete, mirroring every other switch test's synchronisation point.
	// claude must FINISH its turn before the switch terminates it (see awaitTurnComplete).
	awaitTurnComplete(t, h, wsID, chatID, "claude")
	handoff := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	// Switch away — the exact call under test: SwitchProvider now uses
	// TerminateGraceful (SIGTERM + grace, falling back to SIGKILL only if
	// still alive) instead of the old hard Kill for the outgoing claude CLI.
	codexSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)

	codexTermSessID := runnerTerminalSession(t, h, codexSegID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// Switch back to claude immediately — AssembleHandoff only reads
	// Crowbar's own ledger from disk, so this does not depend on codex having
	// bound a session or processed anything; it exercises TerminateGraceful a
	// second time (against codex, which the design doc already established
	// tolerates a hard kill, so this leg is not the interesting one) and then
	// the native --resume path back into claude's original session.
	newClaudeSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)

	newClaudeTermSessID := runnerTerminalSession(t, h, newClaudeSegID)
	require.NotEqual(t, codexTermSessID, newClaudeTermSessID, "switch-back must spawn a new terminal session")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newClaudeTermSessID) })

	newClaudeTap := attachReady(t, h, newClaudeTermSessID, "claude", newClaudeSegID)

	resumedProviderSessionID, resumedRunner := awaitSessionBound(t, h, newClaudeSegID, newClaudeTermSessID, newClaudeTap)
	require.NotEmpty(t, resumedProviderSessionID,
		"the switched-back claude's SessionStart hook never bound: %+v", resumedRunner)
	require.Equal(t, providerSessionID, resumedProviderSessionID,
		"switch-back must --resume the ORIGINAL claude session id (v2's native-resume continuity witness, "+
			"replacing the pre-v2 transcript-path equality)")

	// Drive one follow-up turn to force claude to settle its resumed state —
	// TestAgent_SwitchBackRestoresClaudeContext found live that a synthetic
	// housekeeping turn, when the OLD hard-kill bug produced one, could remain
	// unsurfaced until further input landed, so reading immediately post-resume
	// with no follow-up would not reliably exercise the failure mode this test
	// exists to catch.
	drive(t, h, newClaudeTap, newClaudeTermSessID,
		"What was the codeword I asked you to remember earlier in our conversation? Reply with only that word.")

	texts := awaitAssistantReply(t, h, wsID, chatID, "claude", codeword)
	require.Contains(t, strings.Join(texts, "\n"), codeword,
		"the follow-up turn's reply never referenced the codeword; turns observed: %q", texts)

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

// TestAgent_SwitchBackToCodexResumesItsOwnSession closes the counterpart of gap 4
// for codex, and exists because the app shipped a bug that only a REAL codex could
// show: switching back to codex was impossible.
//
// Crowbar used to point CODEX_HOME at a directory it owned and deleted, which made
// it the custodian of codex's SESSIONS — codex keeps its rollouts inside its home,
// and `codex resume <id>` has nothing else to go on. Leaving codex therefore
// DESTROYED codex's own session: coming back resumed a thread that no longer
// existed, the CLI died on startup ("no rollout found for thread id ..."), its
// segment ended seconds later, and the chat could never return to codex. Every unit
// test passed — the deletion is real and the resume arg is correct; only the vendor
// CLI knows the thread is gone.
//
// Crowbar no longer owns codex's home: a provider owns its own sessions, and Crowbar
// injects its hooks as config overrides instead. This drives that for real — seed
// codex with a codeword, switch to claude, switch BACK — and requires the returning
// codex to (a) resume the SAME native session id and (b) still be ALIVE and
// answering from its own restored session (the gap it is handed carries claude's
// turns, not codex's own).
func TestAgent_SwitchBackToCodexResumesItsOwnSession(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-switch-back", repoPath)

	const codeword = "PANGOLIN-9931"

	// Clear the trust dialog, then DRIVE A TURN, and only then wait for the
	// conversation to bind. The order is forced by codex, not by taste: it creates
	// its rollout — and so fires SessionStart — lazily, on its first real turn.
	// Waiting for a session id on a codex that has not spoken yet waits forever
	// (TestAgent_CodexTurnAppendsLedger drives first, which is why it passes).
	chatID, codexSegID, codexTermSessID, codexTap := spawnReady(t, h, wsID, "codex")

	drive(t, h, codexTap, codexTermSessID, "Remember this exact codeword for the rest of our conversation: "+
		codeword+". Reply with only the word: acknowledged.")

	origSessionID, runner := awaitSessionBound(t, h, codexSegID, codexTermSessID, codexTap)
	require.NotEmpty(t, origSessionID, "codex never bound a session even after a real turn was driven: %+v", runner)

	// codex must FINISH its turn before we leave it. This is THE load-bearing barrier of
	// this test: the switch below terminates the outgoing codex, and if it is still
	// composing its reply, that reply never lands — not in the ledger, and not in codex's
	// own rollout. The codex resumed at the end would then have nothing to recall, and the
	// test would fail on its final assertion for a reason four steps upstream.
	awaitTurnComplete(t, h, wsID, chatID, "codex")
	blob := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, blob, codeword, "codex's turn never reached the ledger")

	// Leave codex. This ends its segment and reaps that segment's tmp dir — the very
	// thing that used to take codex's session with it.
	claudeSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "claude")
	require.NoError(t, err)
	claudeTermSessID := runnerTerminalSession(t, h, claudeSegID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), claudeTermSessID) })
	claudeLegTap := attachReady(t, h, claudeTermSessID, "claude", claudeSegID)
	claudeSessionID, claudeRunner := awaitSessionBound(t, h, claudeSegID, claudeTermSessID, claudeLegTap)
	require.NotEmpty(t, claudeSessionID, "the switched-to claude never bound a session: %+v", claudeRunner)

	// ...and come back.
	backSegID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	backTermSessID := runnerTerminalSession(t, h, backSegID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), backTermSessID) })

	// Drive the follow-up turn FIRST, for the same reason as above: codex announces its
	// conversation on its first turn, not at startup. That turn does double duty — it is
	// also the question whose ANSWER proves the resumed codex still has its own history.
	backTap := attachReady(t, h, backTermSessID, "codex", backSegID)
	drive(t, h, backTap, backTermSessID, "What was the codeword I asked you to remember? Reply with only that word.")

	resumedSessionID, resumedRunner := awaitSessionBound(t, h, backSegID, backTermSessID, backTap)
	require.NotEmpty(t, resumedSessionID,
		"the switched-back codex never bound a session — it almost certainly died on startup, which is "+
			"what happens when its rollout was deleted with the previous runner: %+v", resumedRunner)
	require.Equal(t, origSessionID, resumedSessionID,
		"switching back to codex must RESUME its original native session, not mint a new one")

	replies := awaitAssistantReply(t, h, wsID, chatID, "codex", codeword)
	require.Contains(t, strings.Join(replies, "\n"), codeword,
		"the resumed codex must answer from its OWN restored session; replies seen: %q", replies)
}
