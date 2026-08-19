package agent_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestSubmitPrompt_RejectsNULBeforeJournalOrTUITeardown(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "invalid\x00argv", uuid.NewString())
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Equal(t, spawnCount, f.term.callCount(), "invalid input must not start a replacement")
	assert.Len(t, f.term.terminatedIDs(), terminatedCount, "invalid input must not touch the outgoing TUI")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
	_, statErr := os.Stat(filepath.Join(f.ws.chatsDir, chatID, "prompt-requests"))
	assert.ErrorIs(t, statErr, os.ErrNotExist, "validation must precede durable dispatch intent")
}

func TestSubmitPrompt_ParentDirectorySyncFailureAbortsBeforeTUITeardown(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())
	agentusecase.SetPromptJournalDirSync(f.usecase, func(string) error {
		return errors.New("injected parent fsync failure")
	})

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "must be durable first", uuid.NewString())
	require.Error(t, err)
	assert.NotErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown,
		"the replacement process was never attempted, so this is not an unknown delivery")
	assert.Equal(t, spawnCount, f.term.callCount())
	assert.Len(t, f.term.terminatedIDs(), terminatedCount)
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID, "durability failure must leave the outgoing TUI untouched")
}

func TestSubmitPrompt_FreshLazyCodexNeedsNoBoundSession(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	message := "FIRST REACT MESSAGE"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	require.NotEmpty(t, result.RunnerID)
	require.NotEmpty(t, result.TerminalSessionID)

	call := f.term.calls[f.term.callCount()-1]
	assert.NotContains(t, call.argv, "resume", "a lazy TUI with no announced session is a safe fresh start")
	assert.Equal(t, message, call.argv[len(call.argv)-1], "the completed prompt is one final argv element")
}

func TestSubmitPrompt_ResumeCodexOrdersSubcommandSessionThenPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "native-session")
	turn(t, f, runnerID, "codex", "the current conversation exists")
	message := "CONTINUE FROM REACT"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeAt := indexOf(call.argv, "resume")
	require.GreaterOrEqual(t, resumeAt, 0)
	require.Less(t, resumeAt+3, len(call.argv))
	assert.Equal(t, "native-session", call.argv[resumeAt+1])
	assert.Equal(t, "--", call.argv[resumeAt+2], "the option terminator precedes untrusted positional text")
	assert.Equal(t, message, call.argv[resumeAt+3])
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_FreshClaudeTerminatesVariadicMCPBeforeFinalPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	message := "CLAUDE REACT MESSAGE"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	mcpAt := indexOf(call.argv, "--mcp-config")
	contextAt := indexOf(call.argv, "--append-system-prompt")
	require.GreaterOrEqual(t, mcpAt, 0)
	require.Greater(t, contextAt, mcpAt+1,
		"a following option terminates Claude's variadic --mcp-config before the positional prompt")
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_BlocksNextDispatchUntilUserPromptHook(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", uuid.NewString())
	require.NoError(t, err)
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "two", uuid.NewString())
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"spawn success precedes Working=true; the durable pending request closes that no-hook window")
}

func TestSubmitPrompt_MatchingLateHookFromOutgoingRunnerDoesNotConfirmNewDispatch(t *testing.T) {
	f := newFixture(t)
	chatID, outgoingID := f.spawn(t, "codex")
	f.announce(t, outgoingID, "old-session")
	message := "same text"

	hookDone := make(chan error, 1)
	f.term.duringTerminate = func(string) {
		go func() {
			hookDone <- f.usecase.IngestHook(f.ctx, outgoingID, "codex", "user_prompt",
				mustJSON(t, map[string]any{"prompt": message, "session_id": "old-session"}))
		}()
	}
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()
	f.term.duringTerminate = nil

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString())
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"the old runner's matching hook must not clear the replacement's pending-delivery barrier")
}

func TestSubmitPrompt_ReplacementSpawnFailureStaysOutcomeUnknown(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	f.term.err = errors.New("replacement create failed")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown,
		"a non-command-not-found CreateCommand error may follow a successful fork")
	record, readErr := os.ReadFile(filepath.Join(f.ws.chatsDir, chatID, "prompt-requests", requestID+".json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(record), `"state":"uncertain"`,
		"returning outcome_unknown must release the durable dispatching barrier")

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	assert.ErrorIs(t, retryErr, agentusecase.ErrPromptOutcomeUnknown,
		"outgoing displacement must not mark the blank-runner dispatch safely failed")
}

func TestSubmitPrompt_ReplacementExitBeforeHookStaysOutcomeUnknown(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	require.NoError(t, err)
	f.term.exit(t, result.TerminalSessionID)
	f.wait()

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID)
	assert.ErrorIs(t, retryErr, agentusecase.ErrPromptOutcomeUnknown,
		"process exit can race a hook already in flight, so retrying must not duplicate the prompt")
}

func TestSubmitPrompt_RunnerPersistFailureAfterPTYStartIsOutcomeUnknown(t *testing.T) {
	f, _, runners := newFaultFixture(t)
	chatID, _ := f.spawn(t, "codex")
	runners.failStart = errors.New("runner persistence failed after fork")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", uuid.NewString())
	assert.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	assert.Equal(t, 2, f.term.callCount(), "the replacement PTY started before runner persistence failed")
}

func TestSubmitPrompt_RunnerLookupFailureAndAcceptedCrashGapAreSafe(t *testing.T) {
	f, _, runners := newFaultFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	message := "accepted before response bookkeeping"

	runners.afterStart = func() {
		runners.afterStart = nil
		replacement, err := f.runners.LiveRunnerForChat(f.ctx, chatID)
		require.NoError(t, err)
		require.NoError(t, f.usecase.IngestHook(f.ctx, replacement.ID, "codex", "user_prompt",
			mustJSON(t, map[string]any{"prompt": message})))
		// Fail only the post-spawn lookup. The hook above has already correlated
		// the request as accepted, but markSpawned has not stored the PTY id yet.
		runners.failGet = errors.New("post-spawn runner lookup failed")
		// Startup replay resolves the runner once before and once after acquiring
		// the turn-start interlock. Fail only SubmitPrompt's following result read.
		runners.failGetAfter = 2
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	runners.failGet = nil

	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
	assert.ErrorIs(t, retryErr, agentusecase.ErrPromptAlreadyAccepted,
		"accepted-with-runner but without a committed terminal id must never return a blank success DTO")
}

func TestSubmitPrompt_JournalResultCommitFailureIsOutcomeUnknownAndDoesNotWedgeNewIDs(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	journalDir := filepath.Join(f.ws.chatsDir, chatID, "prompt-requests")
	blockedDir := journalDir + ".blocked"
	f.term.duringFork = func() {
		require.NoError(t, os.Rename(journalDir, blockedDir))
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "commit gap", uuid.NewString())
	f.term.duringFork = nil
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	require.NoError(t, os.Rename(blockedDir, journalDir))

	// The previous request's blank dispatch is normalized to uncertain under the
	// chat gate, so a deliberate new request id is not blocked forever.
	_, retryErr := f.usecase.SubmitPrompt(f.ctx, chatID, "deliberate follow-up", uuid.NewString())
	require.NoError(t, retryErr)
}

func TestReconcileRunnersOnBoot_MarksBlankDispatchIntentUncertain(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	journalDir := filepath.Join(f.ws.chatsDir, chatID, "prompt-requests")
	blockedDir := journalDir + ".blocked"
	f.term.duringFork = func() {
		require.NoError(t, os.Rename(journalDir, blockedDir))
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "crash gap", requestID)
	f.term.duringFork = nil
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	require.NoError(t, os.Rename(blockedDir, journalDir))
	recordPath := filepath.Join(journalDir, requestID+".json")
	before, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	assert.Contains(t, string(before), `"state":"dispatching"`)

	f.term.dieWithDaemon()
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	after, err := os.ReadFile(recordPath)
	require.NoError(t, err)
	assert.Contains(t, string(after), `"state":"uncertain"`,
		"boot must durably release a crash-orphan dispatch barrier")
}

func TestSubmitPrompt_CompletedStoppedResumedChatKeepsNativeResumeIdentity(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "durable-session")
	turn(t, f, runnerID, "codex", "completed before the TUI stopped")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, "term-1")
	f.wait()
	resumedID, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "durable-session", f.runner(t, resumedID).LaunchSessionID)
	f.announce(t, resumedID, "durable-session")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "continue after reopen", uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeAt := indexOf(call.argv, "resume")
	require.GreaterOrEqual(t, resumeAt, 0)
	assert.Equal(t, "durable-session", call.argv[resumeAt+1],
		"launch-as-resume identity wins even though old ledger turns predate the new runner's session_start")
}

func TestSubmitPrompt_NativeTUIResumeOfKnownSessionKeepsContext(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "known-session")
	turn(t, f, runnerID, "codex", "completed in the known conversation")

	// The native TUI opens a new conversation, then /resume selects the older
	// known one. The second announcement moves this same process back to chatID.
	f.announce(t, runnerID, "temporary-new-session")
	f.announce(t, runnerID, "known-session")
	current := f.runner(t, runnerID)
	require.Equal(t, chatID, current.CurrentChatID)
	require.True(t, current.CurrentSessionResumable)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "continue immediately after native resume", uuid.NewString())
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeAt := indexOf(call.argv, "resume")
	require.GreaterOrEqual(t, resumeAt, 0)
	assert.Equal(t, "known-session", call.argv[resumeAt+1])
}

func TestStartupHookBarrier_ReplaysPromptThatFiresBeforeRunnerPersistence(t *testing.T) {
	f := newFixture(t)
	message := "provider fired before runner persistence"
	var earlyRunnerID string
	f.term.duringForkCall = func(call commandCall) {
		earlyRunnerID = segmentIDFromCommand(t, call.argv)
		_, err := f.runners.Get(f.ctx, earlyRunnerID)
		require.Error(t, err, "precondition: the fork callback runs before recordRunner")
		require.NoError(t, f.usecase.IngestHook(f.ctx, earlyRunnerID, "codex", "user_prompt",
			mustJSON(t, map[string]any{"prompt": message})))
	}

	chatID, runnerID := f.spawn(t, "codex")
	f.term.duringForkCall = nil
	assert.Equal(t, runnerID, earlyRunnerID)
	page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 100)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Equal(t, "user", page.Items[0].Role)
	assert.Equal(t, message, page.Items[0].Text)
	assert.True(t, f.chat(t, chatID).Working)
}

func TestSwitchProvider_DoesNotKillPromptAwaitingAcceptanceFromAnotherWindow(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "queued elsewhere", uuid.NewString())
	require.NoError(t, err)
	spawnCount := f.term.callCount()
	terminated := len(f.term.terminatedIDs())

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	assert.Equal(t, spawnCount, f.term.callCount())
	assert.Len(t, f.term.terminatedIDs(), terminated,
		"the replacement TUI awaiting its user_prompt hook must remain alive")
}

func segmentIDFromCommand(t *testing.T, argv []string) string {
	t.Helper()
	const marker = "--segment "
	joined := strings.Join(argv, "\n")
	start := strings.Index(joined, marker)
	require.GreaterOrEqual(t, start, 0, "rendered hook command must carry the runner id")
	fields := strings.Fields(joined[start+len(marker):])
	require.NotEmpty(t, fields)
	return strings.Trim(fields[0], `"'`)
}

func TestSubmitPrompt_ExitAfterStartupBarrierBeforeJournalCommitIsUncertain(t *testing.T) {
	f, _, runners := newFaultFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	runners.afterGet = func(replacement domain.AgentRunner) {
		runners.afterGet = nil
		// SubmitPrompt has returned from spawnRunner (the startup barrier is
		// removed) and has read the replacement, but markSpawned has not run.
		f.term.exit(t, replacement.TerminalSession)
		f.wait()
	}

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "exit in commit gap", requestID)
	require.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	record, readErr := os.ReadFile(filepath.Join(f.ws.chatsDir, chatID, "prompt-requests", requestID+".json"))
	require.NoError(t, readErr)
	assert.Contains(t, string(record), `"state":"uncertain"`,
		"the pre-journaled runner id lets onExit correlate before markSpawned")
}

func TestSubmitPrompt_IdempotentRetryReturnsOriginalSpawnWhilePending(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID)
	require.NoError(t, err)
	retry, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID)
	require.NoError(t, err)
	assert.Equal(t, first, retry)
	assert.Equal(t, 2, f.term.callCount(), "the retry must not spawn a third provider TUI")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "different operation", requestID)
	assert.ErrorIs(t, err, agentusecase.ErrPromptRequestIDConflict)
}

// TestSubmitPrompt_ConcurrentSameRequestIDDeliversOnce is the at-most-once
// property stated as a race rather than as a sequential retry: whatever the
// interleaving of two submissions of one client request id, the prompt is
// delivered once and both callers learn the same outcome.
//
// Delivery is serialised by the chat gate, so in practice the second submission
// meets a completed journal record and returns it. This test does not reach the
// journal's own duplicate report — that branch is unreachable while the gate
// holds — it asserts the property the gate and the journal deliver together.
func TestSubmitPrompt_ConcurrentSameRequestIDDeliversOnce(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	spawnsBefore := f.term.callCount()

	requestID := uuid.NewString()
	const message = "deliver me exactly once"

	type outcome struct {
		dto dto.PromptSubmissionDTO
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			d, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID)
			results <- outcome{dto: d, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	f.wait()

	// At most one replacement CLI: the outgoing one is replaced once, or not at all.
	spawned := f.term.callCount() - spawnsBefore
	assert.LessOrEqual(t, spawned, 1,
		"one request id must never start two replacement CLIs (spawned %d)", spawned)

	// Two successes are legitimate — that is idempotency — but only if they name
	// the SAME delivery. Two different runners would mean the prompt was sent
	// twice under one request id, which is the bug.
	if first.err == nil && second.err == nil {
		assert.Equal(t, first.dto, second.dto,
			"an idempotent retry must return the ORIGINAL delivery, not a second one")
	}
}

// Every guard here runs BEFORE the durable intent and before the live CLI is
// touched, so a rejected prompt leaves the chat exactly as it found it. That is
// what makes these client errors rather than outcome-unknown dispatches.
func TestSubmitPrompt_RejectsBadInputBeforeTouchingAnything(t *testing.T) {
	testCases := []struct {
		name    string
		text    string
		request string
	}{
		{"empty text", "", uuid.NewString()},
		{"whitespace only", "   \n\t ", uuid.NewString()},
		{"oversized text", strings.Repeat("x", 1<<20), uuid.NewString()},
		{"request id is not a uuid", "hello", "not-a-uuid"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, "codex")
			spawns := f.term.callCount()
			terminated := len(f.term.terminatedIDs())

			_, err := f.usecase.SubmitPrompt(f.ctx, chatID, tc.text, tc.request)

			require.ErrorIs(t, err, apperr.ErrInvalidArgument)
			assert.Equal(t, spawns, f.term.callCount(), "no replacement was started")
			assert.Len(t, f.term.terminatedIDs(), terminated, "the live CLI was not touched")
			live, liveErr := f.liveRunnerFor(t, chatID)
			require.NoError(t, liveErr)
			assert.Equal(t, runnerID, live.ID)
		})
	}
}

func TestSubmitPrompt_RefusesAChatThatDoesNotExist(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.SubmitPrompt(f.ctx, uuid.NewString(), "hello", uuid.NewString())

	require.Error(t, err)
}

// A dormant chat has no CLI to replace. Telling the client that specifically is
// what lets the UI offer "resume" instead of a generic failure.
func TestSubmitPrompt_RefusesADormantChat(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptSessionUnavailable)
}

// A provider with no declared prompt-submit capability is TERMINAL-ONLY for this
// operation. That is a capability statement, not a failure, and the client is
// told which so it can point the user at the terminal.
func TestSubmitPrompt_RefusesAProviderWithNoDeclaredDelivery(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(`
id: codex
spawn:
  cmd: /usr/bin/true
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    turn_stop: { message: message }
`), 0o600))
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

// TestRegression_EveryShippedProviderDeliversAPromptByReplacingTheCLI pins the
// ONE delivery channel Crowbar has, on both shipped providers, end to end.
//
// It exists because a second channel was built and then withdrawn. A prompt was
// delivered into the LIVE session through a provider background hook — it worked,
// the process id never changed across deliveries — and it was removed anyway,
// because the wrapper the provider builds around such a payload is visible to the
// model and measurably degraded the answers. The verdict is a product decision
// about output quality, so what has to be defended now is not that the mechanism
// is gone but that its absence costs nothing: a message still reaches the CLI, it
// still reaches it as the FINAL argv element of a REPLACEMENT process, and it is
// still recorded as the user's own words.
//
// codex was never on the withdrawn channel, and is here to say so: its path is the
// path it always had.
func TestRegression_EveryShippedProviderDeliversAPromptByReplacingTheCLI(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, provider)
			turn(t, f, runnerID, provider, "a turn ended")
			spawns := f.term.callCount()
			message := "deliver me by restart"

			result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString())
			require.NoError(t, err)

			require.Equal(t, spawns+1, f.term.callCount(),
				"the message is carried by a process that did not exist before it")
			require.NotEqual(t, runnerID, result.RunnerID,
				"the replacement is a new runner: the CLI holding the chat was replaced")
			call := f.term.calls[f.term.callCount()-1]
			assert.Equal(t, message, call.argv[len(call.argv)-1],
				"the prompt is the final argv element of the replacement")

			// The CLI acknowledging it is what makes the delivery real, and it is the
			// only thing that ever writes the ledger. Recorded as the USER's: no
			// wrapper reaches this path any more, so nothing can reclassify it.
			require.NoError(t, f.usecase.IngestHook(f.ctx, result.RunnerID, provider, "user_prompt",
				mustJSON(t, map[string]any{"prompt": message})))
			f.wait()

			page, err := f.usecase.ReadMessages(f.ctx, chatID, 0, 0, 0)
			require.NoError(t, err)
			require.NotEmpty(t, page.Items)
			last := page.Items[len(page.Items)-1]
			assert.Equal(t, domain.TurnRoleUser, last.Role)
			assert.Equal(t, message, last.Text, "recorded verbatim, with no wrapper to strip")
		})
	}
}
