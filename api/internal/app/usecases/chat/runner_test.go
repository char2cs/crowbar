package chat_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ---------------------------------------------------------------------------
// Resume
// ---------------------------------------------------------------------------

// A chat whose CLI died revives as a switch to the provider last here, which
// resumes its own conversation.
func TestResumeChat_RevivesLastProviderIntoItsOwnConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	exchange(t, f, runnerID, "claude", "claude said something")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI dies
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "revive must bring back the provider that was last here")

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv
	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"revive must resume the CLI's own conversation, not start a blank one")

	// Nothing happened while it was gone, so it is handed no {context} document AT
	// ALL — not an empty one, and not a bare capability preamble. Its own session
	// already holds every turn, and a resume channel is allowed to be a USER MESSAGE
	// (codex's is), so a document with nothing in it that HAPPENED must not be
	// delivered. That rule is provider-independent, which is why claude's silent
	// --append-system-prompt channel is left off here too.
	for _, a := range argv {
		assert.NotEqual(t, "--append-system-prompt", a,
			"a revive with an empty gap has nothing to say and must inject nothing")
		assert.NotContains(t, a, "WHILE YOU WERE AWAY",
			"a revive with an empty gap must hand over no conversation")
		assert.NotContains(t, a, "HANDED-OFF CONTEXT",
			"a revived provider must never be re-fed the conversation it already has")
	}
}

// Reviving a chat revives the provider it was on, even one that never bound a
// conversation; reading only an older provider's row switched the chat back
// with no /switch anywhere.
func TestRegression_ResumeChat_RevivesProviderThatNeverBoundAConversation(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	turn(t, f, claudeRunner, "claude", "claude said something")
	// The switch marker's own timestamp must strictly postdate claude's
	// conversation row, or the two land on the same instant and the comparison
	// this test exists to prove has nothing to distinguish.
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	// No f.announce for codex: it never reports a session_start hook, so it
	// never binds a conversation row — the exact shape the live report's own
	// "ruled out" note describes.

	f.term.exit(t, f.runner(t, codexRunner).TerminalSession) // the CLI dies
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "codex", live.ProviderID,
		"resume must bring back codex — the provider actually running when the chat "+
			"went dormant — not claude, whose conversation is merely older")
}

// waitForClockTick blocks until the wall clock advances: the gap's cut is a
// strict After (turnsAfter), and two turns on one instant would drop the second.
func waitForClockTick(t *testing.T) {
	t.Helper()
	start := time.Now()
	for time.Now().Equal(start) {
		if time.Since(start) > 2*time.Second {
			t.Fatal("real clock never advanced")
		}
	}
}

// claudeAwayOnCodex is a claude chat that spoke, was switched to codex, and
// codex spoke while claude was away; it returns the chat.
func claudeAwayOnCodex(t *testing.T, f testFixture) string {
	t.Helper()
	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude ledger content")
	// The gap's cut is strictly after this turn (turnsAfter): codex's turn
	// must not land on the same instant.
	waitForClockTick(t)

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")
	// codex's turn_stop maps turn.items[type=agentMessage].text (see codex.yaml),
	// not the flat shape turn() builds for claude, which would record nothing.
	codexTurnStop(t, f, codexRunner, "sid-codex-native", "codex spoke while claude was away")
	return chatID
}

// A claude chat resumed into its own conversation is handed what happened while
// it was away as a real user turn: silent config and a get_chat_log pointer are
// both free for the model to ignore.
func TestSwitchProvider_ClaudeSwitchBack_ResumesAndPointsAtTheGap(t *testing.T) {
	f := newFixture(t)

	chatID := claudeAwayOnCodex(t, f)

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"switch-back must resume claude's own conversation, not start a blank one")
	assert.NotContains(t, argv, "--resume sid-claude-native", "the resume flag and its id are separate tokens")

	msg := argv[len(argv)-1]
	assert.Contains(t, msg, "codex spoke while claude was away",
		"the real gap must ride the positional channel directly, not a pointer to fetch it later: %q", msg)
	// <system-reminder> is the tag claude's own harness uses to mark injected
	// content as context rather than instruction — kept regardless of whether
	// this carries a pointer or the real gap, since a bracket-tagged directive
	// with no such wrapper is what read to claude's own safety training as a
	// classic injection attempt in the first place.
	assert.True(t, strings.HasPrefix(msg, "<system-reminder>") && strings.HasSuffix(msg, "</system-reminder>"),
		"the gap must be wrapped as trusted context, not delivered as a bare user turn: %q", msg)
	assert.NotContains(t, msg, "claude ledger content",
		"a provider resumed into its own conversation must not be re-fed its own earlier turns")
	for _, a := range argv {
		assert.NotEqual(t, "--append-system-prompt", a,
			"the gap must ride the same positional channel a real turn does, not silent config")
	}
}

// Switch-then-send through SubmitPrompt: the replacement spawn that delivers the
// text must carry the gap the switch assembled, not recompute that none is owed
// because the session had turned before the switch away.
func TestRegression_SubmitPromptWithStagedProvider_DeliverySpawnCarriesTheGap(t *testing.T) {
	f := newFixture(t)

	chatID := claudeAwayOnCodex(t, f)

	// The combined path: switch back to claude AND deliver a prompt in one
	// call, exactly what the composer does on a staged-provider send.
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	// spawn 0: initial claude. spawn 1: switch to codex. spawn 2: the send's one
	// launch back onto claude, carrying "what did I miss?" — never a silent
	// switch spawn displaced by a second one that carries it.
	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv

	assert.Equal(t, "sid-claude-native", argAfter(t, argv, "--resume"),
		"the delivery spawn must resume claude's own conversation, not start a blank one")

	joined := strings.Join(argv, "\x00")
	assert.Contains(t, joined, "what did I miss?",
		"the delivery spawn must carry the user's actual prompt: argv was %v", argv)
	assert.Contains(t, joined, "codex spoke while claude was away",
		"the delivery spawn must carry the gap claude missed while away: argv was %v", argv)
	assert.NotContains(t, joined, "claude ledger content",
		"a provider resumed into its own conversation must not be re-fed its own earlier turns")

	// Not just present SOMEWHERE in argv — in the SAME trailing positional as
	// the gap, behind exactly one "--". Two SEPARATE positionals (gap, then
	// its own later "-- what did I miss?") is the exact shape that reached
	// claude's real CLI live: it attended to only the first and the second
	// was silently dropped, never even reaching the CLI's own user_prompt
	// hook — see mergeLeadingPositional.
	last := argv[len(argv)-1]
	assert.Contains(t, last, "codex spoke while claude was away")
	assert.Contains(t, last, "what did I miss?")
	assert.Equal(t, 2, len(argv)-slices.Index(argv, "--"),
		"exactly one argv entry must follow the final \"--\": argv was %v", argv)
}

// Echo suppression matches by containment, so a spawn carrying a real prompt
// alongside the injected document must not register it: the user's own message
// would be suppressed with it and never reach the ledger.
func TestRegression_SubmitPromptWithStagedProvider_RealPromptIsRecordedNotSuppressed(t *testing.T) {
	f := newFixture(t)

	chatID := claudeAwayOnCodex(t, f)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	argv := f.term.calls[f.term.callCount()-1].argv
	delivered := argv[len(argv)-1]

	// Exactly what the real claude CLI's own user_prompt hook fires with: the
	// combined positional this spawn was handed, on the session it resumed.
	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": delivered, "session_id": "sid-claude-native"})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "what did I miss?",
		"a spawn carrying a real prompt is never a bare echo — it belongs in the ledger "+
			"like any other user turn, injected preamble and all:\n%s", handoff)
	assert.True(t, f.busy(t, chatID), "the CLI is answering the real prompt: the chat must read as working")
}

// The ledger row for a prompt delivered with injected context holds the user's
// message, not the merged blob: storing the context would fold it into the next
// gap, nesting handoffs without bound.
func TestAudit_SubmitPromptWithStagedProvider_LedgerMustNotPermanentlyStoreTheInjectedContext(t *testing.T) {
	f := newFixture(t)

	chatID := claudeAwayOnCodex(t, f)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	argv := f.term.calls[f.term.callCount()-1].argv
	delivered := argv[len(argv)-1]

	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": delivered, "session_id": "sid-claude-native"})))
	f.wait()

	page, err := f.activity.Turns(f.ctx, chatID)
	require.NoError(t, err)
	var storedUserText string
	for _, item := range page {
		if item.Role == "user" && item.Text != "" && strings.Contains(item.Text, "what did I miss?") {
			storedUserText = item.Text
		}
	}
	require.NotEmpty(t, storedUserText, "the real prompt's own user turn must exist in the ledger")
	assert.Equal(t, "what did I miss?", storedUserText,
		"the permanently stored user turn must be exactly what the person typed — "+
			"not Crowbar's own injected context riding the same spawn, which must never "+
			"become durable ledger content: got %q", storedUserText)
}

// A switch with a model/effort pick riding the same send: the restart the pick
// forces and the gap assembly are separate decisions, and the delivery spawn
// carries both.
func TestRegression_SubmitPromptWithStagedProviderModelAndEffort_GapStillDelivered(t *testing.T) {
	f := newFixture(t)

	chatID := claudeAwayOnCodex(t, f)

	// Switch back to claude, stage a model/effort change, AND deliver a
	// prompt — all in the one call a real staged-everything send makes.
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", &domain.ChatSelection{Model: "opus", Effort: "high"})
	require.NoError(t, err)

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)
	assert.Equal(t, "opus", live.LaunchModel)
	assert.Equal(t, "high", live.LaunchEffort)

	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, "opus", argAfter(t, call.argv, "--model"))

	last := call.argv[len(call.argv)-1]
	assert.Contains(t, last, "codex spoke while claude was away",
		"the gap must still be delivered alongside a staged model/effort change: %v", call.argv)
	assert.Contains(t, last, "what did I miss?")
}

// Sending a dormant chat straight to another provider than it was last on, with
// no Resume first: the switch, the resume checks and the gap all run off durable
// state alone.
func TestRegression_SubmitPromptWithStagedProvider_DormantChatRevivesOntoTheOtherProviderWithTheGap(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude has real history before going dormant")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-native")
	turn(t, f, codexRunner, "codex", "codex spoke, then the chat went dormant")

	f.term.exit(t, f.runner(t, codexRunner).TerminalSession)
	f.wait()
	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I miss?", uuid.NewString(), "claude", nil)
	require.NoError(t, err)

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "claude", live.ProviderID)

	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, "sid-claude-native", argAfter(t, call.argv, "--resume"),
		"reviving straight onto the other provider must still resume its OWN conversation: %v", call.argv)
	last := call.argv[len(call.argv)-1]
	assert.Contains(t, last, "codex spoke, then the chat went dormant",
		"the whole time claude was away — including the gap since before it went dormant — must reach it: %v", call.argv)
	assert.Contains(t, last, "what did I miss?")
}

// TestResumeChat_LiveChat_IsNoop: reviving a chat whose CLI is alive must never tear
// that CLI down — it hands back the runner already on it. (Dormant is a QUERY, so
// "already live" is answerable without any flag.)
func TestResumeChat_LiveChat_IsNoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	got, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, got)
	assert.Equal(t, 1, f.term.callCount(), "a live chat must not respawn its CLI")
	assert.Empty(t, f.term.terminateRequestIDs())
}

// A chat that never announced a conversation still has its own provider, and
// Resume revives that one: refusing it once made the pane start another provider
// on the chat, converting it.
func TestResumeChat_NoConversationButAProviderOfItsOwn_RevivesThatProvider(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "codex", live.ProviderID)
	assert.Equal(t, -1, slices.Index(f.term.calls[1].argv, "resume"),
		"and it comes back FRESH: there is still no conversation to resume into")
}

// A bare row no runner was ever placed on has nothing to resume as: it refuses
// with ErrChatProviderUnknown, since what is missing is the provider, not a runner.
func TestResumeChat_NothingHasEverRunHere_ReturnsError(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.Error(t, err)
	require.ErrorIs(t, err, agentusecase.ErrChatProviderUnknown)
	assert.Contains(t, err.Error(), "no longer records which provider it ran")
	require.ErrorIs(t, err, problem.ErrNotFound,
		"and it still maps to the 404 the old sentinel did, never a 500")
	assert.NotErrorIs(t, err, agentusecase.ErrNoLiveRunner,
		"a missing RUNNER is the expected dormant state, and is not what failed")
}

// codex declares no resume_context_inject (its gap rides the api connection),
// so mergeLeadingPositional never fires for it: argv carries the message alone.
func TestRegression_SubmitPromptWithStagedProvider_CodexTargetUnaffectedByMerge(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	exchange(t, f, claudeRunner, "claude", "claude has real, table-recorded history")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	codexRunner, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	f.announce(t, codexRunner.ID, "sid-codex-native")
	exchange(t, f, codexRunner.ID, "codex", "codex has its own real history now too")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "back to codex now", uuid.NewString(), "codex", nil)
	require.NoError(t, err)

	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, "back to codex now", call.argv[len(call.argv)-1],
		"codex's replacement PTY must carry exactly the message, untouched by the "+
			"positional-merge fix that exists only for claude's own shape: %v", call.argv)
	// The resume subcommand is still there — this fixture has no api connection
	// to take the resume over (see apiResumes), so the PTY carries it. What must
	// NOT appear is a folded-in context document ahead of the message.
	assert.Contains(t, call.argv, "resume",
		"with no api connection, the PTY must still resume codex's own thread: %v", call.argv)
}

// TestSwitchProvider_SwitchBackToProviderWithNoTurns_AssignsANewSession: a provider
// that never spoke here has a session its CLI never wrote, so a switch back assigns
// a new one and hands over the whole conversation.
func TestSwitchProvider_SwitchBackToProviderWithNoTurns_AssignsANewSession(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner := f.spawn(t, "claude")
	// claude binds a conversation but never takes a turn.
	f.announce(t, claudeRunner, "sid-claude-empty")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	turn(t, f, codexRunner, "codex", "codex actually said something")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)

	require.Equal(t, 3, f.term.callCount())
	argv := f.term.calls[2].argv
	assigned := argAfter(t, argv, "--session-id")
	assert.NotEqual(t, "sid-claude-empty", assigned, "an id its CLI never wrote is never handed out again")
	assert.NotContains(t, argv, "--resume", "a session its CLI never wrote cannot be resumed")
	assert.Contains(t, strings.Join(argv, "\x00"), "codex actually said something")
	assert.Equal(t, assigned, f.conversation(t, chatID, "claude").SessionID)
}

// switchResult is what a SwitchProvider driven on its own goroutine hands back.
type switchResult struct {
	runnerID string
	err      error
}

// A switch while the turn runs is refused at once: waiting held the chat's spawn
// gate for as long as the turn lasted, and forcing it lost the answer.
func TestSwitchProvider_MidTurn_IsRefusedAndTouchesNothing(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")

	require.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	f.wait()
	assert.Empty(t, f.term.terminatedIDs(), "the running CLI is left to finish its turn")
	assert.Equal(t, 1, f.term.callCount(), "and no incoming CLI is spawned")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)

	turn(t, f, runnerID, "claude", "the answer the user was waiting for")
	switched, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err, "once the turn ends the switch goes ahead")
	assert.NotEqual(t, runnerID, switched)
}

// TestSwitchProvider_Idle_SwitchesImmediately: the common case. A chat that is not
// working has no turn to refuse on.
func TestSwitchProvider_Idle_SwitchesImmediately(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "a question")
	turn(t, f, runnerID, "claude", "an answer")
	require.False(t, f.busy(t, chatID), "precondition: the chat is idle")

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, f.term.terminatedIDs(), oldTerm)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID)
}

// TestResumeChat_DormantChat_IsNotBusy: a CLI that died mid-turn ended its turn
// with it, so the revive has no running turn to refuse on.
func TestResumeChat_DormantChat_IsNotBusy(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	prompt(t, f, runnerID, "claude", "a question")
	turn(t, f, runnerID, "claude", "an answer")

	// It dies mid-turn: nothing is running, and nothing will ever close this turn.
	prompt(t, f, runnerID, "claude", "and one more thing")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID)
}

// prompt drives a user_prompt hook: the user submitting a message, which is what OPENS a
// turn (the chat goes Working) in production. Every mid-turn test starts here, because
// this hook is the only thing that ever puts a chat mid-turn.
func prompt(
	t *testing.T,
	f testFixture,
	runnerID, provider, message string,
) {
	t.Helper()
	payload := map[string]any{"prompt": message}
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, "user_prompt", mustJSON(t, payload)))
	f.wait()
}

// A /clear mid-turn moves the chat onto a new session; nothing else will close
// the old session's turn, so the change does, or the chat stays busy and the
// workspace spins forever.
func TestRegression_ClearMidTurn_ClosesTheOldSessionsTurn(t *testing.T) {
	f := newFixture(t)

	left, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")

	// The user typed, and then hit /clear before the answer came back.
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.busy(t, left), "precondition: the chat is mid-turn")

	f.announce(t, runnerID, "sid-after-clear") // /clear: the chat moves onto a new session
	f.wait()

	require.Equal(t, left, f.runner(t, runnerID).CurrentChatID, "precondition: a /clear keeps the chat")
	assert.False(t, f.busy(t, left),
		"the turn the old session left open must close: its turn_stop names a session the "+
			"chat is no longer on, so nothing else will ever close it")
}

// A CLI is live before its runner can be recorded, so a record that fails tears
// the CLI down unconditionally and still surfaces the error: a CLI nothing
// points at is invisible.
func TestSwitchProvider_ARunnerThatCannotBeRecordedIsTornDown(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	f.refuseRunnerStarted(t)

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.ErrorContains(t, err, "record runner")

	// term-2 is the codex CLI whose runner could not be recorded.
	require.Equal(t, 2, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-2",
		"the just-spawned CLI whose runner could not be recorded must be torn down")
	_, err = f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "and the chat is free for the next one")
}

func exitDuringFork(f testFixture) {
	f.term.duringForkCall = func(c commandCall) { c.onExit(engineterminal.Exit{}) }
}

func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsIsRefused(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup,
		"a CLI that died before its runner row existed has not started")
	assert.Contains(t, err.Error(), "exited during startup",
		"the refusal has to name what went wrong: the user's own CLI died, and only they can fix it")
}

func TestRegression_ProviderExitingBeforeItsRunnerRowCommitsLeavesNoChat(t *testing.T) {
	f := newFixture(t)
	exitDuringFork(f)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.ErrorIs(t, err, agentusecase.ErrProviderExitedDuringStartup)
	f.wait()

	chats, listErr := f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	assert.Empty(t, chats, "a refused spawn must not leave a chat behind")

	f.term.duringForkCall = nil
	chatID, _ := f.spawn(t, "claude")
	chats, listErr = f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, listErr)
	require.Len(t, chats, 1, "a working provider still creates exactly one chat")
	assert.Equal(t, chatID, chats[0].ID)
}

func TestSlashCatalogPlacementHelperProcess(t *testing.T) {
	started := os.Getenv("CROWBAR_CATALOG_HELPER_STARTED")
	release := os.Getenv("CROWBAR_CATALOG_HELPER_RELEASE")
	if started == "" || release == "" {
		return
	}
	// Announce: the write end blocks until the parent opens the read end.
	announce, err := os.OpenFile(started, os.O_WRONLY, 0)
	if err != nil {
		os.Exit(2)
	}
	_ = announce.Close()
	// Park: the read end blocks until the parent opens the write end.
	parked, err := os.Open(release)
	if err != nil {
		os.Exit(3)
	}
	_ = parked.Close()
	_, _ = fmt.Fprint(os.Stdout, `[{"content":[{"text":"<skills>\n- current: Current skill\n</skills>"}]}]`)
	os.Exit(0)
}

func TestSlashCatalog_MapsEveryEngineFailureToItsOwnAppError(t *testing.T) {
	testCases := []struct {
		name    string
		command string
		want    error
	}{
		{
			name:    "provider command unavailable",
			command: `"/nonexistent/crowbar-not-a-real-cli"`,
			want:    agentusecase.ErrSlashCatalogUnavailable,
		},
		{
			name:    "malformed output",
			command: `"/usr/bin/true"`,
			want:    agentusecase.ErrSlashCatalogMalformed,
		},
		{
			name:    "command failed",
			command: `"/usr/bin/false"`,
			want:    agentusecase.ErrSlashCatalogCommand,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
			require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
			writeCatalogDescriptor(t, f, tc.command)

			chatID, _ := f.spawn(t, "codex")

			_, err := f.usecase.SlashCatalog(f.ctx, chatID)

			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestSlashCatalog_UnsupportedWhenTheProviderDeclaresNoCatalogue(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(`
id: codex
spawn:
  cmd: /usr/bin/true
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
`)), 0o600,
	))

	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogUnsupported)
}

// The probe is its own short-lived command, not a question put to the TUI: a
// dormant chat's catalogue is its provider's, the same as a live one's.
func TestSlashCatalog_ServesADormantChatFromItsOwnProvider(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.MkdirAll(f.ws.worktree, 0o700))
	cli := filepath.Join(t.TempDir(), "cli")
	require.NoError(t, os.WriteFile(cli, []byte("#!/bin/sh\ncat <<'J'\n"+
		`[{"content":[{"text":"<skills>\n- review\n</skills>"}]}]`+"\nJ\n"), 0o700))
	writeCatalogDescriptor(t, f, strconv.Quote(cli))
	chatID, _ := f.spawn(t, "codex")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	catalog, err := f.usecase.SlashCatalog(f.ctx, chatID)

	require.NoError(t, err)
	assert.Equal(t, "codex", catalog.ProviderID)
	require.Len(t, catalog.Items, 1)
	assert.Equal(t, "$review ", catalog.Items[0].InsertText)
}

// SlashCatalog resolves a bubble's cwd through the ancestor walk spawnPaths uses.
// The fake answers every id alike, so only the id that reached it proves it.
func TestSlashCatalog_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")

	_, err := f.usecase.SlashCatalog(f.ctx, bubbleID)

	require.ErrorIs(t, err, agentusecase.ErrSlashCatalogUnsupported,
		"must reach the provider's OWN missing-capability refusal, not a cwd-resolution failure")
	assert.Equal(t, "ws1", f.ws.lastWorkspaceID,
		"must resolve the bubble's cwd through its workspace-owning ancestor, not its own empty WorkspaceID")
}

func writeCatalogDescriptor(t *testing.T, f testFixture, command string) {
	t.Helper()
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(fmt.Sprintf(`
id: codex
spawn:
  cmd: %s
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
presentation:
  slash_catalog:
    completeness: complete
    timeout_ms: 5000
    pipeline:
      adapter: json_text_section
      command: ["--version"]
      text_path: "[].content[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+)$'
      item:
        label: "{name}"
        insert_text: "${name} "
        source: "test"
`, command))), 0o600,
	))
}

func TestSubmitPrompt_RejectsNULBeforeJournalOrTUITeardown(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "invalid\x00argv", uuid.NewString(), "", nil)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	assert.Equal(t, spawnCount, f.term.callCount(), "invalid input must not start a replacement")
	assert.Len(t, f.term.terminatedIDs(), terminatedCount, "invalid input must not touch the outgoing TUI")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
}

func TestSubmitPrompt_FreshLazyCodexNeedsNoBoundSession(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	message := "FIRST REACT MESSAGE"

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
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
	exchange(t, f, runnerID, "codex", "the current conversation exists")
	message := "CONTINUE FROM REACT"

	// codex is api-transport, non-hotswap — but this fixture has no api
	// connection to take the resume over (apiResumes, resume_injection.go), so
	// the PTY is the conversation and its argv must ORDER the resume subcommand,
	// the session id and only then the message: `resume {id} -- <message>`.
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeIdx := slices.Index(call.argv, "resume")
	require.GreaterOrEqual(t, resumeIdx, 0, "argv %v", call.argv)
	require.Less(t, resumeIdx+1, len(call.argv))
	assert.Equal(t, "native-session", call.argv[resumeIdx+1])
	assert.Less(t, resumeIdx, len(call.argv)-1, "the resume subcommand must precede the message")
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_FreshClaudeTerminatesVariadicMCPBeforeFinalPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	message := "CLAUDE REACT MESSAGE"

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	mcpAt := slices.Index(call.argv, "--mcp-config")
	contextAt := slices.Index(call.argv, "--append-system-prompt")
	require.GreaterOrEqual(t, mcpAt, 0)
	require.Greater(t, contextAt, mcpAt+1,
		"a following option terminates Claude's variadic --mcp-config before the positional prompt")
	assert.Equal(t, message, call.argv[len(call.argv)-1])
}

func TestSubmitPrompt_BlocksNextDispatchUntilUserPromptHook(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", uuid.NewString(), "", nil)
	require.NoError(t, err)
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "two", uuid.NewString(), "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"spawn success precedes Working=true; the durable pending request closes that no-hook window")
}

func TestSubmitPrompt_MatchingLateHookFromOutgoingRunnerDoesNotConfirmNewDispatch(t *testing.T) {
	f := newFixture(t)
	chatID, outgoingID := f.spawn(t, "codex")
	f.announce(t, outgoingID, "old-session")
	message := "same text"

	hookDone := make(chan error, 1)
	var once sync.Once
	f.term.duringTerminate = func(string) {
		once.Do(func() {
			hookDone <- f.usecase.IngestHook(f.ctx, outgoingID, "codex", "user_prompt",
				mustJSON(t, map[string]any{"prompt": message, "session_id": "old-session"}))
		})
	}
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()
	f.term.duringTerminate = nil

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptBusy,
		"the old runner's matching hook must not clear the replacement's pending-delivery barrier")
}

func TestSubmitPrompt_RunnerPersistFailureAfterPTYStartIsOutcomeUnknown(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	f.refuseRunnerStarted(t)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", uuid.NewString(), "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptOutcomeUnknown)
	assert.Equal(t, 2, f.term.callCount(), "the replacement PTY started before runner persistence failed")
	_, err = f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "the torn-down replacement leaves the chat free")
}

func TestSubmitPrompt_CompletedStoppedResumedChatKeepsNativeResumeIdentity(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "durable-session")
	exchange(t, f, runnerID, "codex", "completed before the TUI stopped")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.term.exit(t, "term-1")
	f.wait()
	resumedID, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "durable-session", f.conversation(t, chatID, "codex").SessionID)
	f.announce(t, resumedID, "durable-session")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "continue after reopen", uuid.NewString(), "", nil)
	require.NoError(t, err)
	// No api connection in this fixture, so the restart's PTY is the conversation
	// and must resume durable-session itself (apiResumes, resume_injection.go).
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "durable-session", argAfter(t, argv, "resume"))
}

func TestSubmitPrompt_NativeTUIResumeOfKnownSessionKeepsContext(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "known-session")
	exchange(t, f, runnerID, "codex", "completed in the known conversation")

	f.announce(t, runnerID, "temporary-new-session")
	f.announce(t, runnerID, "known-session")
	require.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
	require.Equal(t, "known-session", f.conversation(t, chatID, "codex").SessionID)

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "continue immediately after native resume", uuid.NewString(), "", nil)
	require.NoError(t, err)
	// No api connection in this fixture, so the restart's PTY is the conversation
	// and must resume known-session itself (apiResumes, resume_injection.go) —
	// never temporary-new-session, the one it announced in between.
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "known-session", argAfter(t, argv, "resume"))
}

// A restart_tui provider delivers even the first message after a switch by
// respawning its placeholder; that respawn must carry the handoff the
// placeholder held, or the new provider answers with no history.
func TestSubmitPrompt_VirginNativeSessionAfterSwitchCarriesTheFullHandoff(t *testing.T) {
	f := newFixture(t)
	chatID, codexRunnerID := f.spawn(t, "codex")
	f.announce(t, codexRunnerID, "codex-session")
	// codexTurnStop delivers codex's real api-shaped turn_stop (threadId/
	// turn.items[type=agentMessage].text — see codex.yaml), NOT claude's flat
	// last_assistant_message shape that turn() builds — using turn() here
	// would silently extract an empty message.
	codexTurnStop(t, f, codexRunnerID, "codex-session", "I like turtles")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	claudeRunner, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	// The switch's own placeholder is silent — claude reports its OWN session
	// id the moment it starts, well before the user has said anything to it.
	f.announce(t, claudeRunner.ID, "claude-session")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "what did I say before?", uuid.NewString(), "", nil)
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	contextAt := slices.Index(call.argv, "--append-system-prompt")
	require.GreaterOrEqual(t, contextAt, 0, "the replacement spawn must still carry the handoff")
	require.Less(t, contextAt+1, len(call.argv))
	assert.Contains(t, call.argv[contextAt+1], "I like turtles",
		"the whole prior conversation, not an empty gap, since this native session never turned")
}

// TestSubmitPrompt_AlreadyTurnedNativeSessionStaysOnTheCheapPath is the
// companion regression: a session that has recorded a turn already holds its
// own history, so the fix above must not fire and re-inject a handoff on
// every ordinary follow-up message.
func TestSubmitPrompt_AlreadyTurnedNativeSessionStaysOnTheCheapPath(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "claude-session")
	exchange(t, f, runnerID, "claude", "first answer")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "a normal follow-up", uuid.NewString(), "", nil)
	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, -1, slices.Index(call.argv, "--append-system-prompt"),
		"an already-turned session must not be re-handed a handoff on every follow-up")
}

// A hook the CLI sends before its spawn records runner_started waits on the
// chat's lock, and is recorded on the runner once runner_started is.
func TestSpawn_AHookFromBeforeRunnerStartedIsRecordedAfterIt(t *testing.T) {
	f := newFixture(t)
	message := "provider fired before runner persistence"
	var earlyRunnerID string
	placed := make(chan error, 1)
	f.term.duringForkCall = func(call commandCall) {
		earlyRunnerID = segmentIDFromCommand(t, call.argv)
		early, ok := f.usecase.own.RunnerByID(earlyRunnerID)
		require.True(t, ok, "a spawn's hooks resolve from the first byte")
		st, err := f.ledger.State(f.ctx, early.CurrentChatID)
		require.NoError(t, err)
		_, recorded := st.LiveRunner()
		require.False(t, recorded, "precondition: the fork callback runs before runner_started commits")
		payload := mustJSON(t, map[string]any{"prompt": message})
		go func() { placed <- f.usecase.IngestHook(f.ctx, earlyRunnerID, "codex", "user_prompt", payload) }()
	}

	chatID, runnerID := f.spawn(t, "codex")
	f.term.duringForkCall = nil
	require.NoError(t, <-placed)
	f.wait()
	assert.Equal(t, runnerID, earlyRunnerID)
	page, err := f.activity.Turns(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "user", page[0].Role)
	assert.Equal(t, message, page[0].Text)
	assert.True(t, f.busy(t, chatID))
}

// A switch parked on a prompt another window queued destroys nothing while it
// waits, and the end of its own request ends the wait.
func TestSwitchProvider_DoesNotKillPromptAwaitingAcceptanceFromAnotherWindow(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "queued elsewhere", uuid.NewString(), "", nil)
	require.NoError(t, err)
	spawnCount := f.term.callCount()
	terminated := len(f.term.terminatedIDs())
	parked := parkedOn(t, agentusecase.WaitingForPendingPromptLog)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	go func() {
		select {
		case <-parked:
		case <-ctx.Done():
		}
		cancel()
	}()

	_, err = f.usecase.SwitchProvider(ctx, chatID, "claude")
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, spawnCount, f.term.callCount())
	assert.Len(t, f.term.terminatedIDs(), terminated,
		"the replacement TUI awaiting its user_prompt hook must remain alive")
}

// segmentIDFromCommand is the runner id a CLI's hooks are addressed to: the
// runner-keyed hook URL its argv carries.
func segmentIDFromCommand(t *testing.T, argv []string) string {
	t.Helper()
	m := regexp.MustCompile(`/chats/runners/([^/"]+)/hooks/`).FindStringSubmatch(strings.Join(argv, "\n"))
	require.NotNil(t, m, "rendered hook URL must carry the runner id")
	return m[1]
}

func TestSubmitPrompt_IdempotentRetryReturnsOriginalSpawnWhilePending(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID, "", nil)
	require.NoError(t, err)
	retry, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one operation", requestID, "", nil)
	require.NoError(t, err)
	assert.Equal(t, first, retry)
	assert.Equal(t, 2, f.term.callCount(), "the retry must not spawn a third provider TUI")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "different operation", requestID, "", nil)
	assert.ErrorIs(t, err, agentusecase.ErrPromptRequestIDConflict)
}

func TestSubmitPrompt_ConcurrentSameRequestIDDeliversOnce(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	spawnsBefore := f.term.callCount()

	requestID := uuid.NewString()
	const message = "deliver me exactly once"

	type outcome struct {
		dto domain.AgentPromptSubmission
		err error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			d, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID, "", nil)
			results <- outcome{dto: d, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	f.wait()

	spawned := f.term.callCount() - spawnsBefore
	assert.LessOrEqual(t, spawned, 1,
		"one request id must never start two replacement CLIs (spawned %d)", spawned)

	if first.err == nil && second.err == nil {
		assert.Equal(t, first.dto, second.dto,
			"an idempotent retry must return the ORIGINAL delivery, not a second one")
	}
}

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

			_, err := f.usecase.SubmitPrompt(f.ctx, chatID, tc.text, tc.request, "", nil)

			require.ErrorIs(t, err, problem.ErrInvalidArgument)
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

	_, err := f.usecase.SubmitPrompt(f.ctx, uuid.NewString(), "hello", uuid.NewString(), "", nil)

	require.Error(t, err)
}

// Send is the only intent a client needs: a dormant chat is revived by the
// daemon, through the resume ladder, in the same hold as the delivery.
func TestSubmitPrompt_RevivesADormantChatAndDelivers(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-dormant")
	exchange(t, f, runnerID, "claude", "said before stopping")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	result, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello again", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, result.RunnerID, live.ID)
	argv := f.term.calls[f.term.callCount()-1].argv
	assert.Equal(t, "sid-dormant", argAfter(t, argv, "--resume"), "revived into its own conversation")
	assert.Contains(t, strings.Join(argv, "\x00"), "hello again")
}

// A retry of a request whose outcome is recorded answers from the ledger —
// it never revives, so a retried send cannot spawn a CLI of its own.
func TestSubmitPrompt_ARetryNeverRevives(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	requestID := uuid.NewString()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "once", requestID, "", nil)
	require.NoError(t, err)
	f.wait()
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()
	spawned := f.term.callCount()

	_, _ = f.usecase.SubmitPrompt(f.ctx, chatID, "once", requestID, "", nil)

	assert.Equal(t, spawned, f.term.callCount())
}

func TestSubmitPrompt_RefusesAProviderWithNoDeclaredDelivery(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(f.ws.home, "descriptors"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.home, "descriptors", "codex.yaml"), []byte(withLifecycle(`
id: codex
spawn:
  cmd: /usr/bin/true
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: message
runtime:
  transport: hooks
  hooks:
    format: json
`)), 0o600,
	))
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString(), "", nil)

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

func TestRegression_EveryShippedProviderDeliversAPromptByReplacingTheCLI(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			f := newFixture(t)
			chatID, runnerID := f.spawn(t, provider)
			turn(t, f, runnerID, provider, "a turn ended")
			spawns := f.term.callCount()
			message := "deliver me by restart"

			result, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), "", nil)
			require.NoError(t, err)

			require.Equal(t, spawns+1, f.term.callCount(),
				"the message is carried by a process that did not exist before it")
			require.NotEqual(t, runnerID, result.RunnerID,
				"the replacement is a new runner: the CLI holding the chat was replaced")
			call := f.term.calls[f.term.callCount()-1]
			assert.Equal(t, message, call.argv[len(call.argv)-1],
				"the prompt is the final argv element of the replacement")

			require.NoError(t, f.usecase.IngestHook(f.ctx, result.RunnerID, provider, "user_prompt",
				mustJSON(t, map[string]any{"prompt": message})))
			f.wait()

			page, err := f.activity.Turns(f.ctx, chatID)
			require.NoError(t, err)
			require.NotEmpty(t, page)
			last := page[len(page)-1]
			assert.Equal(t, domain.TurnRoleUser, last.Role)
			assert.Equal(t, message, last.Text, "recorded verbatim, with no wrapper to strip")
		})
	}
}

// compactingDescriptorBody declares the compaction gesture the way claude does: no API
// for it, so the trigger is the slash command injected over the prompt transport.
const compactingDescriptorBody = `
id: claude
display_name: Compacting
spawn:
  cmd: claude
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
  compact_start:
    out: prompt
    send:
      text: "/compact"
runtime:
  transport: hooks
  hooks:
    format: json
`

// The same provider WITHOUT the gesture: everything else identical, so a difference in
// behaviour can only come from compact_start's absence.
const nonCompactingDescriptorBody = `
id: claude
display_name: NotCompacting
spawn:
  cmd: claude
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

// Compaction reaches the CLI as the provider's OWN gesture — Crowbar cannot compact
// anything itself, the context belongs to the provider.
func TestCompact_SendsTheProvidersDeclaredGesture(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))

	call := f.term.calls[f.term.callCount()-1]
	joined := strings.Join(call.argv, " ")
	assert.Contains(t, joined, "/compact",
		"the provider's declared gesture must reach the CLI verbatim")
}

// Key-presence IS the capability. A provider that declares no gesture cannot be asked,
// and must say so rather than silently doing nothing — a compact button that appears
// to work and does not is worse than one that is absent.
func TestCompact_AProviderWithNoGestureIsNotFound(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	before := f.term.callCount()
	err := f.usecase.Compact(f.ctx, chatID)

	require.Error(t, err)
	require.ErrorIs(t, err, problem.ErrNotFound)
	assert.Equal(t, before, f.term.callCount(),
		"a provider that cannot compact must have nothing sent to it")
}

// A chat that does not exist must fail before anything is sent anywhere.
func TestCompact_AnUnknownChatSendsNothing(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingDescriptorBody)

	before := f.term.callCount()
	err := f.usecase.Compact(f.ctx, "no-such-chat")

	require.Error(t, err)
	assert.Equal(t, before, f.term.callCount(),
		"an unknown chat must reach no CLI at all")
}

// Compact resolves a bubble's cwd through the ancestor walk. The error alone
// cannot show it (a refusal is also ErrNotFound), so the resolved workspace id
// is asserted.
func TestCompact_ResolvesCwdThroughTheAncestorWalkForABubble(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", nonCompactingDescriptorBody)
	bubbleID, _, _ := seedBubbleChat(t, f, "claude")

	err := f.usecase.Compact(f.ctx, bubbleID)

	require.ErrorIs(t, err, problem.ErrNotFound,
		"must still refuse for the provider's OWN missing gesture, exactly as an ordinary chat would")
	assert.Equal(t, "ws1", f.ws.lastWorkspaceID,
		"must resolve the bubble's cwd through its workspace-owning ancestor, not its own empty WorkspaceID")
}

// The same provider as compactingDescriptorBody, with compact_pre/compact_post
// also mapped — mirroring claude.yaml, which maps them to
// PreCompact/PostCompact with the same {session_id, trigger} fields.
const compactingWithHooksDescriptorBody = `
id: claude
display_name: Compacting
spawn:
  cmd: claude
  interactive_required: true
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
  compact_start:
    out: prompt
    send:
      text: "/compact"
  compact_pre:
    in: PreCompact
    map:
      session_id: session_id
      trigger: trigger
  compact_post:
    in: PostCompact
    map:
      session_id: session_id
      trigger: trigger
runtime:
  transport: hooks
  hooks:
    format: json
`

// TestRegression_CompactPreSettlesThePendingDeliveryImmediately: /compact is an
// ordinary prompt no user_prompt hook acknowledges, and compact_post often never
// arrives, so compact_pre settles it as consumed by the provider's builtin.
func TestRegression_CompactPreSettlesThePendingDeliveryImmediately(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingWithHooksDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: compact is delivered as an ordinary prompt, with nothing yet confirming it")

	// SubmitPrompt's restart_tui strategy REPLACES the runner to deliver
	// "/compact" — the delivery's own RunnerID (and the compact hooks that
	// follow) belong to that replacement, not the one f.spawn returned.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "compact_pre",
		mustJSON(t, map[string]any{"session_id": "native-session", "trigger": "manual"})))

	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"compact_pre alone must settle the delivery, not leave it for compact_post")
	prompts := crowbarPrompts(t, f, chatID)
	require.Len(t, prompts, 1)
	assert.True(t, prompts[0].Consumed, "the CLI ran its own builtin")
	assert.Equal(t, ledger.ReasonProviderBuiltin, prompts[0].Reason)
}

// TestRegression_CompactPostAlsoSettlesADeliveryCompactPreDidNot is the
// defensive twin: if compaction ever happens with a turn genuinely open (so
// compact_pre's Interrupt call does NOT resolve instantly), compact_post must
// still settle the delivery when it does arrive.
func TestRegression_CompactPostAlsoSettlesADeliveryCompactPreDidNot(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", compactingWithHooksDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")

	require.NoError(t, f.usecase.Compact(f.ctx, chatID))
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)

	// No compact_pre delivered at all here — standing in for the "not idle"
	// case (a turn open when compaction starts), which this test does not
	// need to construct in full: it only needs SOMETHING left pending when
	// compact_post arrives.
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID))

	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "claude", "compact_post",
		mustJSON(t, map[string]any{"session_id": "native-session", "trigger": "manual"})))

	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"compact_post must settle a delivery compact_pre left pending")
}

// No carrier from before a restart can acknowledge a prompt, so boot settles
// every pending one rather than leave its chat busy.
func TestReconcileRunnersOnBoot_SettlesEveryPendingPromptAsDaemonRestart(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	requestID := uuid.NewString()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "lost to the restart", requestID, "", nil)
	require.NoError(t, err)
	f.restart(t)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))

	got := receipt(t, f, chatID, requestID)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonDaemonRestart, got.Reason)
}

// TestRegression_DeadPTY_MeansDeadRunner: no CLI outlives the daemon that
// started it, so boot ends every runner the ledger still holds live — and
// leaves the chat, whose ledger is what Resume revives from.
func TestRegression_DeadPTY_MeansDeadRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	f.restart(t)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "no runner may outlive its PTY")
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, runnerID, exit.RunnerID)
	assert.Equal(t, domain.AgentExitDaemonRestart, exit.Reason)

	// ...but the chat is still resumable: its ledger describes what HAPPENED, which no
	// restart can falsify.
	require.Equal(t, "s1", f.conversation(t, chatID, "claude").SessionID)
}

// TestRegression_AfterRestart_ResumeStillWorks: a chat whose CLI died with the
// daemon comes back as a NEW runner, never as the id of the dead one.
func TestRegression_AfterRestart_ResumeStillWorks(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	turn(t, f, runnerID, "claude", "the reply the user came back for")

	f.restart(t)

	// The ledger still holds the runner live: its exit happened in a process
	// that was gone before it could record it.
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	stale, ok := st.LiveRunner()
	require.True(t, ok, "precondition: the pre-restart runner outlives the daemon in the ledger")
	require.Equal(t, runnerID, stale.RunnerID)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)

	assert.NotEqual(t, runnerID, revived,
		"Resume must spawn a NEW runner, not hand back the id of a CLI that died with the daemon")
	assert.Equal(t, 2, f.term.callCount(), "Resume must actually launch a vendor CLI")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID, "and the chat is held by the CLI the user can actually talk to")
}

// TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever: a CLI that dies
// mid-turn never sends its turn_stop, and after a restart no exit callback is
// left to close it; boot does, since its runner is gone.
func TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "a long-running task the daemon died in the middle of"})))
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn")

	f.restart(t)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.False(t, f.busy(t, chatID),
		"a chat that was mid-turn when the daemon died must not spin forever")
}

// TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir: on a clean exit the onExit callback
// removes the runner's tmp dir. A crash is precisely the case where that callback never ran,
// so these dirs are the one orphan class that would otherwise accumulate forever.
func TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	require.DirExists(t, tmpDir, "precondition: the spawned CLI has a tmp dir")

	f.restart(t) // the daemon died: onExit never fired, so the dir was never removed
	require.DirExists(t, tmpDir, "precondition: a crashed daemon reaps nothing on its way out")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.NoDirExists(t, tmpDir, "boot reconcile must reap the tmp dir of a CLI that died with the daemon")
}

// TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer: on an idle machine nothing is running,
// and "nothing is running" is a real answer, not a failure.
func TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
}

// A chat with no workspace (a bubble, model spec §3.1) must not abort the boot
// sweep for every other chat (spec §1.5).
func TestReconcileRunnersOnBoot_WorkspacelessChatDoesNotBreakTheSweep(t *testing.T) {
	f := newFixture(t)

	bubbleID, err := f.usecase.MintChat(f.ctx, "", "", "", "")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx),
		"a workspace-less chat must not fail the boot sweep for every chat")

	_, err = f.usecase.GetChat(f.ctx, bubbleID)
	require.NoError(t, err)
}

// Closing a chat tab stops its CLI and leaves the chat dormant with its bound
// conversation, so ResumeChat revives the real conversation.
func TestStopChat_TerminatesTheCLI_LeavesChatDormantAndResumable(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	require.NotEmpty(t, oldTerm)

	// A real, resumable conversation: the CLI announced its native session and took a
	// turn (a session id is not a conversation until something is written to it).
	f.announce(t, runnerID, "sid-claude-native")
	turn(t, f, runnerID, "claude", "claude said something")

	// And it is mid-answer when the user closes the tab — the state whose spinner the
	// close must clear.
	prompt(t, f, runnerID, "claude", "and one more thing")
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	// (1) the vendor CLI was gracefully terminated.
	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "close must gracefully terminate the live CLI")

	// (2) the chat is DORMANT — no runner points at it any more.
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "after close the chat has no live runner")

	// (3) the chat still EXISTS, and its spinner was cleared (the aborted turn).
	assert.NotEmpty(t, f.chat(t, chatID).ID)
	assert.False(t, f.busy(t, chatID), "closing mid-turn must clear the spinner, not leave it spinning forever")

	// (4) its bound conversation is retained — this is what makes it resumable.
	assert.Equal(t, "sid-claude-native", f.conversation(t, chatID, "claude").SessionID,
		"a close must keep the conversation it can be resumed into")

	// The close returned once the CLI's exit was recorded, as a stop.
	assert.False(t, f.isLive(runnerID))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, domain.AgentExitStopped, exit.Reason)

	// (5) reopening revives the REAL conversation: the last provider, resumed into its
	// own native session, exactly where the user left it.
	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "claude", live.ProviderID, "revive brings back the provider that was last here")

	require.Equal(t, 2, f.term.callCount(), "resume spawns a fresh CLI on the same chat")
	assert.Equal(t, "sid-claude-native", argAfter(t, f.term.calls[1].argv, "--resume"),
		"revive must resume the CLI's OWN conversation, not start a blank one")
}

// TestStopChat_AbortsInFlightTurn_DoesNotWait is the user's explicit choice made testable:
// "close = stop immediately". StopChat terminates the CLI mid-answer and clears the
// spinner right away; a straight-line call that returns is the proof.
func TestStopChat_AbortsInFlightTurn_DoesNotWait(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	oldTerm := f.runner(t, runnerID).TerminalSession
	prompt(t, f, runnerID, "claude", "think hard about this")
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	assert.Contains(t, f.term.terminateRequestIDs(), oldTerm, "the mid-turn CLI is terminated — the abort is intended")
	assert.False(t, f.busy(t, chatID), "the aborted turn's spinner is cleared at once")
}

// TestStopChat_AlreadyDormant_IsNilNoop: a chat whose CLI is already gone has nothing to
// stop, so the close is a clean no-op — never an error, and it terminates nothing.
func TestStopChat_AlreadyDormant_IsNilNoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI exits on its own
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID), "stopping an already-dormant chat is a nil no-op")
	f.wait()

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	assert.NotEmpty(t, f.chat(t, chatID).ID, "the chat is still there — a no-op close does not remove it")
}

// TestStopChat_ATerminateThatFailsKeepsTheRunner: a CLI the terminate could not
// end is still running, so the chat keeps it — live, not stuck mid-exit, so a
// resume answers with it and a later stop can still end it.
func TestStopChat_ATerminateThatFailsKeepsTheRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.ErrorContains(t, f.usecase.StopChat(f.ctx, chatID), "terminate outgoing terminal")

	bounded, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	resumed, err := f.usecase.ResumeChat(bounded, chatID)
	require.NoError(t, err, "a runner left exiting would park the resume")
	assert.Equal(t, runnerID, resumed, "the chat still belongs to the CLI that would not die")

	f.term.terminateErr = nil
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	exit, ok := f.lastExit(t, chatID)
	require.True(t, ok)
	assert.Equal(t, domain.AgentExitStopped, exit.Reason)
}

// TestStopChat_DoesNotDeleteTheChat pins the boundary against PurgeChat: a close STOPS the
// process but must never Forget the chat — it stays in the read model, addressable and
// resumable, exactly the difference between closing a tab and deleting a chat.
func TestStopChat_DoesNotDeleteTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	got := f.chat(t, chatID)
	assert.Equal(t, chatID, got.ID, "a close must not delete the chat")

	all, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	var found bool
	for _, c := range all {
		if c.ID == chatID {
			found = true
		}
	}
	assert.True(t, found, "the stopped chat still appears in the chat list")
}

// ---------------------------------------------------------------------------
// The bugs. One test per bug, each named for what it locks out.
// ---------------------------------------------------------------------------

// TestRegression_ClearKeepsTheChatAndThePTY: /clear (the CLI announces a session the
// chat never had) moves the chat onto that session and marks the change — same chat,
// same runner, same PTY.
func TestRegression_ClearKeepsTheChatAndThePTY(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "s1")
	exchange(t, f, r1, "codex", "said on s1")
	before := f.runner(t, r1)

	f.announce(t, r1, "s2") // /clear: a conversation nobody has seen

	after := f.runner(t, r1)
	assert.Equal(t, chatA, after.CurrentChatID, "a /clear never takes the CLI off its chat")
	assert.Equal(t, before.TerminalSession, after.TerminalSession, "the PTY must not change on a /clear")
	assert.Equal(t, 1, f.term.callCount(), "no new CLI may be spawned for a /clear")
	assert.Empty(t, f.term.terminateRequestIDs(), "and nothing may be terminated")

	assert.Equal(t, "s2", f.conversation(t, chatA, "codex").SessionID)
	assert.Equal(t, []string{"session_changed:s2"}, f.markers(t, chatA))
	assert.Equal(t, chatA, f.chatForSession(t, "codex", "s1"), "the session it left stays the chat's own")
}

// TestRegression_ResumeOfAnotherChatsSession_IsAConflictNotAMove: a CLI that /resumes a
// session another chat owns, from a chat holding a conversation of its own, takes
// nothing: its chat keeps its binding, says so, and its CLI is ended rather than
// left running where nothing it does is recorded.
func TestRegression_ResumeOfAnotherChatsSession_IsAConflictNotAMove(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "sA")
	exchange(t, f, r1, "codex", "said on sA")
	terminal := f.runner(t, r1).TerminalSession
	chatB, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "sB")

	f.announce(t, r1, "sB")

	assert.Equal(t, "sA", f.conversation(t, chatA, "codex").SessionID, "its chat keeps its own session")
	assert.Equal(t, []string{"session_conflict:sB"}, f.markers(t, chatA))
	assert.Equal(t, chatB, f.chatForSession(t, "codex", "sB"))
	assert.Equal(t, r2, mustLive(t, f, chatB).ID, "nobody is evicted")
	require.Eventually(t, func() bool { return !f.isLive(r1) }, 5*time.Second, time.Millisecond,
		"the CLI that left its chat's conversation is ended")
	assert.Equal(t, []string{terminal}, f.term.terminateRequestIDs())
	exit, ok := f.lastExit(t, chatA)
	require.True(t, ok)
	assert.Equal(t, domain.AgentExitDisplaced, exit.Reason)
	assert.True(t, f.conversation(t, chatA, "codex").Resumable(), "and the next send resumes it")
}

// A chat whose conversation was lost is relaunched on a stand-in session. A
// /resume typed there lands the picked conversation in that chat: the claim
// moves, the chat that held it loses it and its CLI is ended.
func TestRegression_ResumeInARelaunchedChat_BindsTheConversationInPlace(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	exchange(t, f, r1, "claude", "the conversation the user wants back")
	terminal := f.runner(t, r1).TerminalSession

	chatB, lost := f.spawn(t, "claude")
	f.announce(t, lost, "sLost")
	exchange(t, f, lost, "claude", "history Crowbar kept but claude lost")
	f.term.exit(t, f.runner(t, lost).TerminalSession)
	f.wait()
	refused, err := f.usecase.ResumeChat(f.ctx, chatB)
	require.NoError(t, err)
	f.wait()
	f.term.exitWith(t, f.runner(t, refused).TerminalSession, engineterminal.Exit{
		Code: 1, Screen: "No conversation found with session ID: sLost",
	})
	f.wait()
	require.Equal(t, ledger.ConversationLost, f.conversation(t, chatB, "claude").State)

	r2, err := f.usecase.ResumeChat(f.ctx, chatB)
	require.NoError(t, err)
	f.wait()
	f.announce(t, r2, f.conversation(t, chatB, "claude").SessionID)

	f.announce(t, r2, "sA")
	got := f.conversation(t, chatB, "claude")
	assert.Equal(t, "sA", got.SessionID)
	assert.Equal(t, ledger.CauseMoved, got.Cause)
	assert.Equal(t, chatB, f.chatForSession(t, "claude", "sA"), "the claim moved with it")
	assert.Equal(t, ledger.ConversationLost, f.conversation(t, chatA, "claude").State, "the chat it left lost it")
	require.Eventually(t, func() bool { return !f.isLive(r1) }, 5*time.Second, time.Millisecond,
		"one CLI per session: the one on the chat that gave it up is ended")
	assert.Contains(t, f.term.terminateRequestIDs(), terminal)
	exit, ok := f.lastExit(t, chatA)
	require.True(t, ok)
	assert.Equal(t, domain.AgentExitMoved, exit.Reason)
	for _, chatID := range []string{chatA, chatB} {
		rows, err := f.ledger.Read(f.ctx, chatID, 0, 0)
		require.NoError(t, err)
		replayed, _ := ledger.Fold(chatID, rows).Conversation("claude")
		assert.Equal(t, f.conversation(t, chatID, "claude"), replayed, "the move replays from the rows alone")
	}

	dead := deadLetters(t, f)
	exchange(t, f, r2, "claude", "recorded where the user resumed it")
	page, err := f.activity.Turns(f.ctx, chatB)
	require.NoError(t, err)
	assert.Contains(t, textsOfPage(page), "recorded where the user resumed it")
	assert.Equal(t, dead, deadLetters(t, f), "nothing the CLI sends is dead-lettered")
}

// ---------------------------------------------------------------------------
// Spawn
// ---------------------------------------------------------------------------

func TestSpawnChat_CreatesChatAndRunner_AndSpawnsTheCLI(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)

	chat := f.chat(t, chatID)
	assert.Equal(t, "ws1", chat.WorkspaceID)

	r := f.runner(t, runnerID)
	assert.Equal(t, "claude", r.ProviderID)
	assert.Equal(t, chatID, r.CurrentChatID)
	assert.Equal(t, "ws1", r.WorkspaceID)
	assert.NotEmpty(t, r.TerminalSession)
	assigned := f.conversation(t, chatID, "claude").SessionID
	assert.NotEmpty(t, assigned, "claude is launched on a session id Crowbar minted for the chat")

	// The chat is live because a runner points at it — not because of a flag on it.
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	assert.Equal(t, chatID, call.chatID,
		"the CLI's PTY session is owned by the chat that spawned it")
	assert.Equal(t, f.ws.worktree, call.cwd)
	assert.Equal(t, "claude", filepath.Base(call.argv[0]))
	assert.Equal(t, assigned, argAfter(t, call.argv, "--session-id"))
	// A fresh SpawnChat injects the capability preamble via the descriptor's
	// context_inject mechanism (claude.yaml maps it to --append-system-prompt); it
	// must be present, not the raw ledger handoff (there is none yet for a
	// brand-new chat).
	assert.Contains(t, call.argv, "--append-system-prompt")
}

// TestSpawnRunner_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenThePTYDies guards the
// resource-leak fix: the per-spawn tmp dir (the rendered hook config the CLI is pointed
// at) must still exist while that CLI runs, and be removed when its PTY dies.
func TestSpawnRunner_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenThePTYDies(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	// <chatsDir>/runners/<runnerID>-<provider>: keyed by the RUNNER, not the chat, so the
	// dir stays findable from a bare runner row even after Displace erases its chat pointer
	// (see worktreepath.RunnerDir).
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	info, err := os.Stat(tmpDir)
	require.NoError(t, err, "the tmp dir must exist immediately after spawn")
	assert.True(t, info.IsDir())

	require.Equal(t, 1, f.term.callCount())
	require.NotNil(t, f.term.calls[0].onExit, "CreateCommand must receive a non-nil onExit")

	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "and must survive while the CLI is running")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)

	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "the tmp dir must be removed once the PTY dies")
}

func TestSpawnChat_UsesDescriptorCmdAsArgv0(t *testing.T) {
	for _, providerID := range []string{"claude", "codex"} {
		t.Run(providerID, func(t *testing.T) {
			f := newFixture(t)

			_, runnerID := f.spawn(t, providerID)
			require.NotEmpty(t, runnerID)

			require.Equal(t, 1, f.term.callCount())
			// Basename, not the whole token: argv[0] is run through binpath.Resolve,
			// so it is the CLI's absolute path wherever the binary is installed and
			// the bare name only when it is nowhere to be found.
			assert.Equal(t, providerID, filepath.Base(f.term.calls[0].argv[0]))
		})
	}
}

// argv[0] is exec'd by absolute path (binpath.Resolve): a launchd daemon's
// minimal PATH misses where the CLIs install.
func TestSpawnChat_ResolvesArgv0ToAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))
	// The CLI is reachable ONLY through this dir, exactly as it is reachable only
	// through ~/.local/bin in the failing install.
	t.Setenv("PATH", dir)

	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	require.NotEmpty(t, runnerID)

	require.Equal(t, 1, f.term.callCount())
	assert.Equal(t, bin, f.term.calls[0].argv[0])
}

func TestSpawnRunner_CrowbarBinaryPathFallsBackToHomeBinCrowbar(t *testing.T) {
	f := newFixture(t)
	home := f.ws.home

	f.spawn(t, "claude")

	require.Equal(t, 1, f.term.callCount())
	mcpConfig := argAfter(t, f.term.calls[0].argv, "--mcp-config")
	assert.Contains(t, mcpConfig, filepath.Join(home, "bin", "crowbar"),
		"the tool bridge still runs the binary installed under the home")
	data, err := os.ReadFile(argAfter(t, f.term.calls[0].argv, "--settings"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "crowbar hook", "hooks no longer exec the binary per event")
}

// TestSpawn_HookConfigCarriesRunnerAndProvider guards the arg-based spawn attribution:
// the RUNNER id (the crowbarSegmentID every hook carries) and the provider are
// rendered into the hook URLs of the hook config, not injected as an env var.
func TestSpawn_HookConfigCarriesRunnerAndProvider(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	settingsPath := argAfter(t, call.argv, "--settings")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "/chats/runners/"+runnerID+"/hooks/claude/")

	for _, kv := range call.env {
		assert.False(t, strings.HasPrefix(kv, "CROWBAR_SEGMENT_ID="), "env must not carry CROWBAR_SEGMENT_ID: %q", kv)
	}
}

// ---------------------------------------------------------------------------
// session_start — a bind, a change, a conflict, or nothing
// ---------------------------------------------------------------------------

func TestIngestHook_SessionStart_Bind_RecordsTheConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.bc.reset()
	f.snapFrames.reset()

	f.announce(t, runnerID, "sid-abc")

	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID, "a bind stays put: the runner does not move")
	c := f.conversation(t, chatID, "codex")
	assert.Equal(t, "sid-abc", c.SessionID)
	assert.False(t, c.BoundAt.IsZero(), "the conversation is stamped with when it opened")

	// And the session is now OWNED: a later announcement of it elsewhere is a conflict.
	assert.Equal(t, chatID, f.chatForSession(t, "codex", "sid-abc"))
	assert.Empty(t, f.markers(t, chatID), "binding an unbound chat is not a change")

	assert.Empty(t, f.runnerKinds(t), "the session is the chat's, never the runner's")
	assert.Empty(t, f.bcKinds(t), "binding a conversation writes nothing to the chat")
}

// TestIngestHook_SessionStart_FirstAnnouncementOfAKnownID_BindsInPlace: a resumed spawn
// (ResumeChat / a switch-back) comes up already inside the conversation it was told to
// resume; announcing it changes nothing.
func TestIngestHook_SessionStart_FirstAnnouncementOfAKnownID_BindsInPlace(t *testing.T) {
	f := newFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	exchange(t, f, r1, "claude", "content so sA is resumable")
	f.term.exit(t, f.runner(t, r1).TerminalSession) // the CLI exits; chat A goes dormant
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatA)
	require.NoError(t, err)
	f.wait()

	f.announce(t, revived, "sA")

	assert.Equal(t, chatA, f.runner(t, revived).CurrentChatID, "the resumed runner stays on the chat it was spawned into")
	assert.Equal(t, "sA", f.conversation(t, chatA, "claude").SessionID)
	assert.Empty(t, f.markers(t, chatA),
		"the first announcement replaced a session that never turned, and the revive's is the bound one")
	assert.Empty(t, f.term.terminateRequestIDs(), "and nothing is evicted: the runner did not move")
}

func TestIngestHook_SessionStart_SameConversationIsANoop(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.snapFrames.reset()

	f.announce(t, runnerID, "sid-1")
	f.announce(t, runnerID, "sid-1")

	assert.Empty(t, f.runnerKinds(t), "announcing a conversation issues no runner command at all")
	assert.Empty(t, f.markers(t, chatID), "re-announcing the same conversation changes nothing")
	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
}

func TestIngestHook_SessionStart_EmptySessionID_IsRefused(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.snapFrames.reset()

	refusedHook(t, f, runnerID, "", "session_start", map[string]any{"session_id": ""})

	assert.Empty(t, f.runnerKinds(t), "an announcement with no conversation id is not an event")
	assert.Equal(t, chatID, f.runner(t, runnerID).CurrentChatID)
}

// ---------------------------------------------------------------------------
// Turn hooks
// ---------------------------------------------------------------------------

func TestIngestHook_TurnStopAppendsAssistantTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "do the thing")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "done thing"})))

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "assistant (claude): done thing")

	// turn_stop ends the turn; recording the answer emits no frame of its own.
	assert.Equal(t, []string{"turn_stopped"}, f.bcKinds(t))
}

// TestIngestHook_UserPromptAppendsUserTurn: user_prompt appends a user turn, fires the
// derived-title fallback first (empty title), and opens the turn.
func TestIngestHook_UserPromptAppendsUserTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "please do the thing"})))

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "user: please do the thing")

	// The derived title (the chat record) and the turn's start (the ledger)
	// each emit exactly one frame, on its own feed.
	kinds := f.bcKinds(t)
	assert.Equal(t, []string{"title_set"}, inFeed(kinds, recordFeed...))
	assert.Equal(t, []string{"turn_started"}, inFeed(kinds, ledgerFeed...))
}

// TestIngestHook_TurnStop_EmptyMessage_AppendsNoLedgerTurnButStillClosesTurn: an empty
// message is a ledger no-op, not a turn-state no-op.
func TestIngestHook_TurnStop_EmptyMessage_AppendsNoLedgerTurnButStillClosesTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	prompt(t, f, runnerID, "claude", "ask")
	f.bc.reset()

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{})))

	assert.Equal(t, []string{"turn_stopped"}, f.bcKinds(t))

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "assistant (claude)", "an empty assistant message must append no ledger turn")
}

// TestIngestHook_UserPromptOpensTurn_TurnStopClosesTurn: the hooks drive the chat's
// live turn state, which is what the workspace's working spinner reads.
func TestIngestHook_UserPromptOpensTurn_TurnStopClosesTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.False(t, f.busy(t, chatID), "a fresh chat is not busy")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "start working"})))
	assert.True(t, f.busy(t, chatID), "a user_prompt must open the turn")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "done"})))
	assert.False(t, f.busy(t, chatID), "a turn_stop must close the turn")
}

// ---------------------------------------------------------------------------
// Runner exit
// ---------------------------------------------------------------------------

// TestOnExit_DeadPTY_MeansDeadRunner: the PTY is the sole authority on liveness. When
// it dies the runner is gone from the live model — and the chat is dormant, not broken.
func TestOnExit_DeadPTY_MeansDeadRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	assert.False(t, f.isLive(runnerID), "no runner may outlive its PTY")
	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "the chat is dormant")

	// ...but its conversation survives the process, so it stays resumable.
	assert.Equal(t, "s1", f.conversation(t, chatID, "claude").SessionID)
}

// TestOnExit_MidTurn_ClosesTheTurn: a CLI that dies mid-turn never sends its turn_stop,
// so without this the chat would spin forever.
func TestOnExit_MidTurn_ClosesTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "do work"})))
	require.True(t, f.busy(t, chatID), "precondition: the turn is open")

	f.term.exit(t, f.runner(t, runnerID).TerminalSession)

	assert.False(t, f.busy(t, chatID), "a dead CLI cannot still be mid-turn")
}

// TestOnExit_AfterSwitch_DoesNotCloseTheIncomingRunnersTurn: a provider switch starts
// the incoming CLI while the outgoing one is still dying (SIGTERM is not synchronous),
// so the outgoing runner's belated exit must not close the INCOMING runner's turn.
func TestOnExit_AfterSwitch_DoesNotCloseTheIncomingRunnersTurn(t *testing.T) {
	f := newFixture(t)

	chatID, oldRunner := f.spawn(t, "claude")
	oldTerm := f.runner(t, oldRunner).TerminalSession

	newRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(f.ctx, newRunner, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the new CLI is working"})))
	require.True(t, f.busy(t, chatID), "precondition: the INCOMING runner has an open turn")

	// The outgoing CLI finally dies.
	f.term.exit(t, oldTerm)
	f.wait()

	assert.True(t, f.busy(t, chatID),
		"the outgoing runner's exit must not close the incoming runner's turn")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, newRunner, live.ID, "and the incoming runner still holds the chat")
}

// ---------------------------------------------------------------------------
// Broadcast
// ---------------------------------------------------------------------------

// recordFeed and ledgerFeed are the chat frame kinds each feed publishes.
var (
	recordFeed = []string{"created", "permission_level_set", "title_set", "deleted"}
	ledgerFeed = []string{"turn_started", "turn_stopped"}
)

// inFeed is kinds kept to feed's, in order: frames are ordered within one feed.
func inFeed(kinds []string, feed ...string) []string {
	return slices.DeleteFunc(slices.Clone(kinds), func(k string) bool { return !slices.Contains(feed, k) })
}

// TestBroadcast_ChatAndRunnerFramesAreDistinct pins the single-broadcaster invariant
// across both aggregates: each event produces exactly one frame on its own feed, in
// order, and a session change produces neither — it is the chat ledger's alone.
func TestBroadcast_ChatAndRunnerFramesAreDistinct(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hello"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "hi"})))
	f.wait()
	f.announce(t, runnerID, "s2") // /clear: the chat moves onto a new session
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	// The chat record and the ledger are two feeds, so only each one's own
	// frames are ordered.
	kinds := f.bcKinds(t)
	assert.Equal(t, []string{
		"created",              // SpawnChat
		"permission_level_set", // SpawnChat: seeded from the global default
		"title_set",            // user_prompt: derived title
		"deleted",              // PurgeChat (asynx Forget's OnForget)
	}, inFeed(kinds, recordFeed...))
	assert.Equal(t, []string{"turn_started", "turn_stopped"}, inFeed(kinds, ledgerFeed...))

	assert.Equal(t, []string{
		"started", // SpawnChat
		"exited",  // PurgeChat
	}, f.runnerKinds(t))
}

// ---------------------------------------------------------------------------
// Error paths
// ---------------------------------------------------------------------------

func TestSpawnChat_CreateChatFailure_TearsDownCLIAndWraps(t *testing.T) {
	f, cs := newFaultFixture(t)
	cs.failCreate = fmt.Errorf("boom: create")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create chat")

	// The CLI was already spawned (term-1) and must be torn down so no orphan process
	// leaks when the persist fails.
	require.Equal(t, 1, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-1")
}

// TestSpawnChat_StartRunnerFailure_TearsDownCLIAndWraps: a CLI whose runner cannot be
// recorded is invisible to Crowbar. Kill it rather than leak it.
func TestSpawnChat_StartRunnerFailure_TearsDownCLIAndWraps(t *testing.T) {
	f := newFixture(t)
	f.refuseRunnerStarted(t)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.ErrorContains(t, err, "record runner")

	require.Equal(t, 1, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-1",
		"a CLI nothing points at must never be left running")
}

// A spawn refused after the chat was written (a CLI exiting at startup, a runner row
// that will not commit) forgets the chat. The frames prove it was created first.
func TestRegression_SpawnRefusedAfterTheChatWasWritten_ForgetsTheChat(t *testing.T) {
	for _, tc := range []struct {
		name string
		arm  func(t *testing.T, f testFixture)
	}{
		{
			// The production case: `true` as a vendor CLI — a process that swears it is
			// interactive and is already gone by the time its row could be written.
			name: "the CLI exits during startup",
			arm: func(t *testing.T, f testFixture) {
				t.Helper()
				f.term.duringForkCall = func(call commandCall) { call.onExit(engineterminal.Exit{}) }
			},
		},
		{
			name: "the runner row will not commit",
			arm: func(t *testing.T, f testFixture) {
				t.Helper()
				f.refuseRunnerStarted(t)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.arm(t, f)

			chatID, runnerID, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
			require.Error(t, err)
			assert.Empty(t, chatID, "a refused create hands back no chat")
			assert.Empty(t, runnerID)
			f.wait()

			// Forget supersedes frames still queued, so the middle one may be gone.
			assert.Contains(t, [][]string{{"created", "deleted"}, {"created", "permission_level_set", "deleted"}},
				f.bcKinds(t), "the chat was written and then taken back out — not merely never written")

			var born string
			for _, frame := range f.bc.snapshot() {
				if frame.kind == "created" {
					born = frame.chatID
				}
			}
			require.NotEmpty(t, born, "precondition: the spawn got far enough to write a chat")

			_, getErr := f.chats.GetChat(f.ctx, born)
			assert.ErrorIs(t, getErr, agentchat.ErrNotFound,
				"the aggregate is Forgotten, not just hidden from the list")
			listed, err := f.chats.ListChats(f.ctx)
			require.NoError(t, err)
			assert.Empty(t, listed, "a refused spawn must not leave a chat behind")
		})
	}
}

func TestSpawnChat_CreateCommandFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.term.err = fmt.Errorf("boom: create command")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create command")
}

func TestSpawnChat_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.ws.err = fmt.Errorf("boom: worktree lookup")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestSpawnChat_UnknownProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
}

func TestIngestHook_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", "session_start",
		mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestIngestHook_ChatLookupFailure_ReturnsWrappedError(t *testing.T) {
	f, cs := newFaultFixture(t)

	_, runnerID := f.spawn(t, "claude")

	cs.failGetChat = fmt.Errorf("boom: get chat")
	err := f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hi"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hook: chat")
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

func TestListChats_GetChat_ConversationsForChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "s1")

	chats, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	assert.Equal(t, chatID, chats[0].ID)

	chat, err := f.usecase.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, chatID, chat.ID)

	convs, err := f.usecase.ConversationsForChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, convs, 1)
	assert.Equal(t, "s1", convs[0].SessionID)
	assert.Equal(t, "codex", convs[0].ProviderID)

	live, err := f.usecase.LiveRunnerForChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, runnerID, live.ID)
}

// argAfter returns the argv token following flag.
func argAfter(t *testing.T, argv []string, flag string) string {
	t.Helper()
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	t.Fatalf("flag %q not found in argv %v", flag, argv)
	return ""
}

// configValue returns the value of the `-c <prefix>...` override a codex spawn was
// given. codex takes several -c flags (hooks, update check, the injected context), so
// "the token after the first -c" is not good enough.
func configValue(t *testing.T, argv []string, prefix string) string {
	t.Helper()
	for i, a := range argv {
		if a == "-c" && i+1 < len(argv) && strings.HasPrefix(argv[i+1], prefix) {
			return argv[i+1]
		}
	}
	t.Fatalf("no -c %s... override in argv %v", prefix, argv)
	return ""
}

// TestSpawnChat_MissingCLI_PropagatesCommandNotFound: when the vendor CLI is not
// installed, the sentinel must survive the usecase intact. Burying it in a generic
// wrap chain is what mapped it to an opaque 500, so the UI had nothing to say and the
// chat button just did nothing.
func TestSpawnChat_MissingCLI_PropagatesCommandNotFound(t *testing.T) {
	f := newFixture(t)
	f.term.err = fmt.Errorf("%w: claude", engineterminal.ErrCommandNotFound)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.Error(t, err)
	require.ErrorIs(t, err, engineterminal.ErrCommandNotFound,
		"the not-installed fact must reach the handler, which maps it to 424")
	assert.Contains(t, err.Error(), "claude", "and it must name the provider the UI has to report")
}

// The answer is recorded with the turn's end, ahead of it: the chat view reads
// the record once on that edge, so a later append would strand the reply.
func TestRegression_TurnStop_AssistantMessageIsDurableBeforeTheTurnEnds(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "ask"})))
	f.wait()

	var atEnd []ledger.Row
	f.ledger.Subscribe(func(id string, rows []ledger.Row, _ ledger.State) {
		if end := slices.IndexFunc(rows, func(r ledger.Row) bool { return r.Kind == ledger.KindTurnEnded }); id == chatID && end >= 0 {
			atEnd = rows[:end]
		}
	})

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the answer"})))
	f.wait()

	said := slices.IndexFunc(atEnd, func(r ledger.Row) bool { return r.Kind == ledger.KindAssistantText })
	require.GreaterOrEqual(t, said, 0, "the reply must ride the commit that ends the turn, ahead of its end")
	body, err := ledger.DecodeBody[ledger.AssistantText](atEnd[said])
	require.NoError(t, err)
	assert.Equal(t, "the answer", body.Text)
}

// TestRegression_TurnStop_EmptyMessageStillClosesTheTurn guards the invariant the old
// ordering existed to protect: an empty assistant message is a ledger no-op, but must
// never be a turn-state no-op. Appending first must not make the turn depend on there
// being something to append.
func TestRegression_TurnStop_EmptyMessageStillClosesTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "ask"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{})))
	f.wait()

	assert.False(t, f.busy(t, chatID), "an empty turn_stop must still close the turn")
}

func TestListChatsByWorkspace_ReturnsOnlyThatWorkspacesChats(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	mine, err := f.usecase.ListChatsByWorkspace(f.ctx, "ws1")
	require.NoError(t, err)
	ids := make([]string, 0, len(mine))
	for _, c := range mine {
		ids = append(ids, c.ID)
	}
	assert.Contains(t, ids, chatID)

	other, err := f.usecase.ListChatsByWorkspace(f.ctx, "ws-elsewhere")
	require.NoError(t, err)
	assert.Empty(t, other)
}

// A retry after a lost HTTP response must receive the ORIGINAL delivery, even
// though that delivery has since made the chat busy. Reporting "busy" instead
// would tell a client its message failed when it had already been sent.
func TestSubmitPrompt_ARetryAfterAConfirmedDeliveryReturnsTheOriginal(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	const message = "the prompt that landed"
	requestID := uuid.NewString()

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID, "", nil)
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(f.ctx, live.ID, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": message, "transcript_path": "/rollouts/x"})))
	f.wait()
	spawnsBefore := f.term.callCount()

	second, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, requestID, "", nil)

	require.NoError(t, err)
	assert.Equal(t, first, second, "a retry reports the delivery that already happened")
	assert.Equal(t, spawnsBefore, f.term.callCount(), "and starts no second CLI")

	turns, err := f.activity.Turns(f.ctx, chatID)
	require.NoError(t, err)
	assert.Len(t, turns, 1, "the prompt is recorded once")
}

// Each prompt and catalogue refusal carries its own code, wrapped or not, so
// the client branches on the code rather than the prose.
func TestPromptAndCatalogRefusals_CarryDistinctCodes(t *testing.T) {
	testCases := []struct {
		err  error
		want problem.Code
	}{
		{agentusecase.ErrPromptBusy, problem.ChatBusy},
		{agentusecase.ErrPromptOutcomeUnknown, problem.RequestOutcomeUncertain},
		{agentusecase.ErrPromptRequestIDConflict, problem.RequestIDConflict},
		{agentusecase.ErrPromptUnsupported, problem.PromptSubmitUnsupported},
		{agentusecase.ErrPromptSessionUnavailable, problem.LiveTUIRequired},
		{agentusecase.ErrSlashCatalogUnsupported, problem.CatalogUnsupported},
		{agentusecase.ErrSlashCatalogTimeout, problem.CatalogTimeout},
		{agentusecase.ErrSlashCatalogUnavailable, problem.CatalogCommandUnavailable},
		{agentusecase.ErrSlashCatalogOutputLimit, problem.CatalogOutputLimit},
		{agentusecase.ErrSlashCatalogCommand, problem.CatalogCommandFailed},
		{agentusecase.ErrSlashCatalogMalformed, problem.CatalogMalformedOutput},
		{agentusecase.ErrSlashCatalogSuperseded, problem.CatalogSuperseded},
	}
	seen := map[problem.Code]struct{}{}
	for _, tc := range testCases {
		t.Run(string(tc.want), func(t *testing.T) {
			assert.Equal(t, tc.want, problem.CodeOf(tc.err))
			assert.Equal(t, tc.want, problem.CodeOf(fmt.Errorf("agent: submit prompt: %w", tc.err)))
		})
		_, dup := seen[tc.want]
		assert.False(t, dup, "codes must be distinct: %s", tc.want)
		seen[tc.want] = struct{}{}
	}
}

// A coded refusal still matches its category, which callers test with errors.Is.
func TestPromptRefusals_StillMatchTheirCategory(t *testing.T) {
	require.ErrorIs(t, agentusecase.ErrPromptBusy, problem.ErrConflict)
	require.ErrorIs(t, agentusecase.ErrPromptUnsupported, problem.ErrUnprocessable)
	require.ErrorIs(t, agentusecase.ErrSlashCatalogUnavailable, engineterminal.ErrCommandNotFound)
}

const trustDialog = "❯ 1. Yes, I trust this folder\n  2. No, exit\n  Enter to confirm · Esc to cancel"

func TestUsecase_MatchTerminalPrompt_ResolvesTheShippedDescriptor(t *testing.T) {
	f := newFixture(t)

	prompt, ok := f.usecase.MatchTerminalPrompt(f.ctx, "claude", trustDialog)

	require.True(t, ok)
	assert.Equal(t, domain.AgentTerminalWaitTrust, prompt.Kind)
}

func TestUsecase_MatchTerminalPrompt_UnknownProviderIsSilent(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalPrompt(f.ctx, "telepathy", trustDialog)

	assert.False(t, ok)
}

func TestUsecase_MatchTerminalPrompt_OrdinaryScreenIsNotABlock(t *testing.T) {
	f := newFixture(t)

	_, ok := f.usecase.MatchTerminalPrompt(f.ctx, "claude", "> Ready.\n  shift+tab to cycle")

	assert.False(t, ok)
}

// deltaHook renders one claude `message_delta` payload exactly as the bundled
// descriptor declares it (session_id/turn_id/message_id/index/final/delta), so
// the real engine parses it rather than a shape invented here.
func deltaHook(t *testing.T, messageID string, index int, final bool, text string) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"turn_id":    "turn-1",
		"message_id": messageID,
		"index":      index,
		"final":      final,
		"delta":      text,
	})
}

// Reasoning streams live only: it is never written to the ledger as said text.
//
// Payload shape is a LIVE capture against codex-cli 0.149.1 — see
// engine/agents/.../testdata/fixtures/codex/item_reasoning_summaryTextDelta.json.
func TestRegression_ReasoningStreamsLiveAndIsNeverRecorded(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")

	think := func(index int, text string) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHook(
			f.ctx, runnerID, "codex", "reasoning_delta",
			mustJSON(t, map[string]any{
				"threadId": "sess-1", "turnId": "turn-1", "itemId": "rs_1",
				"summaryIndex": index, "delta": text,
			})))
	}
	think(0, "**Clarifying** ")
	think(0, "the wording")
	f.wait()

	turns, err := f.activity.Turns(f.ctx, chatID)
	require.NoError(t, err)
	for _, tn := range turns {
		assert.NotContains(t, tn.Text, "Clarifying",
			"reasoning must never be written to the ledger")
	}
}

// A streamed message is recorded once, whole, when it completes.
func TestIngestHook_AStreamedMessageIsRecordedWhole(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "say it"})))

	post := func(index int, final bool, text string) {
		t.Helper()
		require.NoError(t, f.usecase.IngestHook(
			f.ctx, runnerID, "claude", "message_delta",
			deltaHook(t, "msg-one", index, final, text),
		))
	}
	post(0, false, "THE ")
	post(1, false, "MESSAGE ")
	post(2, true, "SO FAR")
	f.wait()

	page, err := f.activity.Turns(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, page, 2, "the prompt, then the reply")
	assert.Equal(t, "THE MESSAGE SO FAR", page[1].Text)
}

// Two concurrent sends, each staging its own provider on one chat, never
// cross-deliver: each message lands on the provider its own call staged
// (SubmitPromptWithSwitch switches and delivers under one gate).
func TestRegression_SubmitPromptWithStagedProvider_ConcurrentSendsNeverCrossDeliver(t *testing.T) {
	f := newFixture(t)
	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude")

	type outcome struct {
		provider string
		message  string
		result   domain.AgentPromptSubmission
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	launch := func(provider, message string) {
		go func() {
			<-start
			d, err := f.usecase.SubmitPrompt(f.ctx, chatID, message, uuid.NewString(), provider, nil)
			results <- outcome{provider: provider, message: message, result: d, err: err}
		}()
	}
	launch("codex", "message meant for codex")
	launch("claude", "message meant for claude")
	close(start)
	first := <-results
	if first.err == nil {
		// The later send's switch waits for this prompt: the provider takes it and answers.
		prompt(t, f, first.result.RunnerID, first.provider, first.message)
		turn(t, f, first.result.RunnerID, first.provider, "done")
	}
	deadline, cancel := context.WithTimeout(f.ctx, 10*time.Second)
	defer cancel()
	var second outcome
	select {
	case second = <-results:
	case <-deadline.Done():
		t.Fatal("the later send never finished")
	}
	f.wait()

	startedAs := map[string]string{}
	for _, row := range ledgerRows(t, f, chatID) {
		if row.Kind == ledger.KindRunnerStarted {
			started, err := ledger.DecodeBody[ledger.RunnerStarted](row)
			require.NoError(t, err)
			startedAs[started.RunnerID] = started.Provider
		}
	}
	for _, o := range []outcome{first, second} {
		if o.err != nil {
			// A refusal is never a misdelivery — only a wrong destination is
			// the bug this test exists to catch.
			continue
		}
		assert.Equal(t, o.provider, startedAs[o.result.RunnerID],
			"a staged-provider send must land on the provider IT staged, never a "+
				"concurrent caller's: message=%q landed on provider=%s", o.message, startedAs[o.result.RunnerID])
	}
}

// Targets claude for the invalid model: codex's catalogue is discovered from a
// live CLI, so only claude's static list can call a model invalid here.
func TestRegression_SubmitPromptWithStagedProviderAndInvalidModel_SwitchStaysCommitted(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "claude", &domain.ChatSelection{Model: "not-a-real-model", Effort: ""})

	require.Error(t, err)
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	t.Logf("after invalid-model SubmitPrompt with a staged switch: err=%v liveProvider=%s working=%v",
		err, live.ProviderID, f.busy(t, chatID))
	assert.Equal(t, "codex", live.ProviderID,
		"a switch committed by a call whose OWN selection step then fails should not silently "+
			"strand the chat on the new provider with no prompt delivered and no way back but another switch")
}

// A send the gate turns away must not leave its staged model/effort behind:
// the NEXT send would otherwise restart the provider for a pick that was
// never delivered.
func TestRegression_SubmitPromptWithStagedSelection_BusyRejectionChangesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", uuid.NewString(), "", nil)
	require.NoError(t, err)
	before := f.chat(t, chatID)

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "two", uuid.NewString(), "",
		&domain.ChatSelection{Model: "staged-model", Effort: "high"})

	require.ErrorIs(t, err, agentusecase.ErrPromptBusy)
	after := f.chat(t, chatID)
	assert.Equal(t, before.Model, after.Model)
	assert.Equal(t, before.Effort, after.Effort)
}

func TestRegression_SubmitPromptWithStagedSelection_ReplayedRequestChangesNothing(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "codex")
	requestID := uuid.NewString()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "one", requestID, "", nil)
	require.NoError(t, err)
	before := f.chat(t, chatID)

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "one", requestID, "",
		&domain.ChatSelection{Model: "staged-model", Effort: "high"})

	require.NoError(t, err)
	after := f.chat(t, chatID)
	assert.Equal(t, before.Model, after.Model)
	assert.Equal(t, before.Effort, after.Effort)
}

// ─── the prompt a departing runner leaves behind ──────────────

// TestRegression_AChatWhoseCLIClearedIsNotLeftOwingAPrompt: a CLI that moves the
// chat onto a new session can no longer acknowledge the prompt it carried in the
// old one, so the change must settle it — else the chat answers 409 to every
// later prompt.
func TestRegression_AChatWhoseCLIClearedIsNotLeftOwingAPrompt(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "conversation-one")
	exchange(t, f, runnerID, "codex", "hello")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "can you call some subagents?", uuid.NewString(), "", nil)
	require.NoError(t, err)
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	f.announce(t, live.ID, "conversation-one") // the delivery resumed it
	require.True(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"precondition: the delivery is spawned and nothing has confirmed it")

	// The user typed /clear: the CLI moves this chat onto a session nobody has seen.
	f.announce(t, live.ID, "conversation-three")
	require.Equal(t, chatID, f.runner(t, live.ID).CurrentChatID, "precondition: the runner stayed")

	assert.False(t, agentusecase.HasPendingPrompt(f.usecase.RunnerUsecase, f.ctx, chatID),
		"the change must settle the delivery the old session took with it")
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "and now?", uuid.NewString(), "", nil)
	require.NoError(t, err, "the chat must not be left owing an answer nobody can give")
}

// TestRegression_DormantChatWithAPromptFromAVanishedRunnerStillResumes: a
// departure that never settled its prompt leaves one no runner can acknowledge.
// The pending check settles it rather than refuse every resume for good.
func TestRegression_DormantChatWithAPromptFromAVanishedRunnerStillResumes(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "conversation-one")
	turn(t, f, runnerID, "codex", "hello")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()
	orphan := plantPrompt(t, f, chatID, "a-runner-that-no-longer-exists")

	_, err := f.usecase.ResumeChat(f.ctx, chatID)

	require.NoError(t, err, "a prompt whose carrier is gone must not refuse the resume forever")
	got := receipt(t, f, chatID, orphan)
	assert.Equal(t, ledger.PromptStatusSettled, got.Status)
	assert.Equal(t, ledger.ReasonRunnerExited, got.Reason)
}

// The same orphan must not hold a live chat busy for its next prompt either.
func TestRegression_APromptFromAVanishedRunnerDoesNotHoldTheNextPromptBusy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude-native")
	orphan := plantPrompt(t, f, chatID, "a-runner-that-no-longer-exists")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "and now?", uuid.NewString(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, ledger.ReasonRunnerExited, receipt(t, f, chatID, orphan).Reason)
}

// ---------------------------------------------------------------------------
// A chat never changes provider without being asked
// ---------------------------------------------------------------------------

// Re-opening a dormant chat born on codex keeps codex: it wrote no conversation
// row and no switch marker, and resolving that absence to the first enabled
// provider changed the chat on a click.
func TestRegression_ReopeningADormantChatBornOnCodexKeepsCodex(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	// No f.announce: nothing ever binds a conversation row for this chat, and
	// there is no earlier provider's row to inherit either.
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner, "precondition: the chat is dormant")
	convs, err := f.usecase.ConversationsForChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Empty(t, convs, "precondition: codex bound nothing")
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Empty(t, ints, "precondition: a chat born on codex carries no switch marker")

	// The chat itself is the only thing left that knows.
	assert.Equal(t, "codex", f.chat(t, chatID).ProviderID,
		"a chat must remember the vendor it was born on, with or without a runner on it")

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err,
		"re-opening this chat must revive it, not fail and leave the UI to guess a provider")
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID)
	assert.Equal(t, "codex", live.ProviderID,
		"the chat came back as codex — a reopen that converts it to another vendor is the bug")
}

// A switch back to the chat's own provider records provider_switched: the
// durable choice is what makes the previous provider knowable.
func TestRegression_SwitchingBackToTheChatsOwnProviderKeepsRecordingTheMarker(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1, "a real conversion must leave a breadcrumb even with nothing bound")
	assert.Equal(t, ledger.MarkerProviderSwitched, ints[0].Kind)
	assert.Equal(t, "claude", ints[0].Detail)
	assert.Equal(t, "claude", f.chat(t, chatID).ProviderID,
		"and the chat's own durable choice moves with the switch the user asked for")
}

// Switching a chat with no resolvable provider (a row from before chats carried
// one) still leaves the switch marker: the conversion is never silent.
func TestRegression_SwitchProvider_ConvertingAChatWithNoResolvableProviderStillLeavesAMarker(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.NoError(t, err)
	f.wait()
	require.Empty(t, f.chat(t, chatID).ProviderID,
		"precondition: nothing about this chat names a vendor, exactly like a pre-field row")

	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1, "a conversion with an unknown starting point is still a conversion")
	assert.Equal(t, ledger.MarkerProviderSwitched, ints[0].Kind)
	assert.Equal(t, "claude", ints[0].Detail)
}

// A revive resumes the chat's own session by its id, and the chat says which one.
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

// A session the chat never turned on was never written by its CLI: every launch
// until it turns is assigned a new id, never its old one and never resumed.
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

// A provider that takes no id at launch starts fresh on a session that never
// turned; its announcement rebinds the chat with no change, so the prompt the
// launch carries stays owed and is delivered.
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

// A resume the CLI itself refuses loses the session: the chat says why its runner
// ended, and the next launch starts a fresh session with no transcript.
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
	assert.NotContains(t, strings.Join(argv, "\x00"), "before the corruption", "no transcript is handed over")

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "after the loss", uuid.NewString(), "", nil)
	require.NoError(t, err)
	f.wait()
	argv = f.term.calls[f.term.callCount()-1].argv
	assert.NotEqual(t, fresh.SessionID, argAfter(t, argv, "--session-id"), "the fresh session never turned")
	assert.Equal(t, ledger.CauseFresh, f.conversation(t, chatID, "claude").Cause, "and its replacement is fresh too")
	assert.Contains(t, strings.Join(argv, "\x00"), "after the loss")
	assert.NotContains(t, strings.Join(argv, "\x00"), "before the corruption", "nor to the launch carrying the prompt")
}

// The prompt that launched a resume the CLI refuses is lost with the session,
// and nothing launches again to redeliver it.
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

// A resumed CLI Crowbar stops did not refuse its session: the chat says it was
// stopped, and the next revive resumes it.
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
	f.term.exit(t, result.TerminalSessionID) // the stopped CLI goes
	f.wait()
	assert.Equal(t, domain.AgentExitStopped, agentusecase.ChatExitReason(f.usecase.RunnerUsecase, chatID))

	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	assert.Equal(t, "sid-kept", argAfter(t, f.term.calls[f.term.callCount()-1].argv, "--resume"),
		"a session nobody refused is never lost")
}

// A replacement that cannot start leaves the chat dormant saying so — never a
// silent dormancy, and never the "displaced" of the runner it replaced.
func TestSessionExit_AReplacementThatCannotStartRecordsSpawnFailed(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-before")
	turn(t, f, runnerID, "claude", "an answer")
	f.term.err = errors.New("pty: out of descriptors")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "next", uuid.NewString(), "", nil)
	require.Error(t, err)

	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
	assert.Equal(t, domain.AgentExitSpawnFailed, agentusecase.ChatExitReason(f.usecase.RunnerUsecase, chatID))
}

// sessionOp is one step of the invariant walk: anything a user, a CLI or the
// vendor's own store can do to a chat.
type sessionOp struct {
	name string
	do   func(t *testing.T, f testFixture, w *sessionWalk)
}

// sessionWalk is the chat the walk is on and what it has seen so far.
type sessionWalk struct {
	chatID   string
	provider string
	sessions int
	// rival is the intent racing this one, when two run at once.
	rival string
}

// live never quiesces the model: a racing op reads it while its rival acts.
func (w *sessionWalk) live(f testFixture) (agents.Runner, bool) {
	r, err := f.usecase.own.LiveRunnerForChat(f.ctx, w.chatID)
	return r, err == nil
}

var sessionOps = []sessionOp{
	{"send", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		sub, err := f.usecase.SubmitPrompt(f.ctx, w.chatID, "step", uuid.NewString(), "", nil)
		if err != nil {
			return
		}
		// S5: a delivered send has a runner to continue the conversation on —
		// unless a rival stop or crash took the chat once the send released it.
		assert.NotEmpty(t, sub.RunnerID, "S5: a delivered send names no runner")
		live, ok := w.live(f)
		assert.True(t, ok || endsRunner(w.rival), "S5: a delivered send left the chat with no runner")
		// The CLI takes what it is sent; a switch parked on the prompt waits for that.
		if ok && live.ID == sub.RunnerID {
			err := f.usecase.IngestHook(f.ctx, live.ID, live.ProviderID, "user_prompt", mustJSON(t, map[string]any{"prompt": "step"}))
			if !endsRunner(w.rival) || !errors.Is(err, problem.ErrNotFound) { // a rival stop may take the runner first
				assert.NoError(t, err)
			}
		}
	}},
	{"stop", stopOp},
	{"switch", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		w.provider = map[string]string{"claude": "codex", "codex": "claude"}[w.provider]
		_, _ = f.usecase.SwitchProvider(f.ctx, w.chatID, w.provider)
	}},
	{"resume", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		_, _ = f.usecase.ResumeChat(f.ctx, w.chatID)
	}},
	{"crash", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if r, ok := w.live(f); ok && r.TerminalSession != "" && f.term.SessionLive(f.ctx, r.TerminalSession) {
			f.term.exit(t, r.TerminalSession)
		}
	}},
	{"restart", func(t *testing.T, f testFixture, _ *sessionWalk) {
		t.Helper()
		f.restart(t)
		require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	}},
	{"announce", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if r, ok := w.live(f); ok {
			w.sessions++
			f.announce(t, r.ID, fmt.Sprintf("walk-%s-%d", w.chatID[:8], w.sessions))
		}
	}},
	{"prompt", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		// A turn the CLI opened and has not finished: whatever comes next must close it.
		if r, ok := w.live(f); ok && f.sessions[r.ID] != "" {
			payload := map[string]any{"prompt": "typed in the terminal"}
			require.NoError(t, f.usecase.IngestHook(f.ctx, r.ID, r.ProviderID, "user_prompt", mustJSON(t, payload)))
		}
	}},
	{"turn", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if r, ok := w.live(f); ok && f.sessions[r.ID] != "" {
			turn(t, f, r.ID, r.ProviderID, "an answer")
		}
	}},
	{"refuse", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		// The CLI dies refusing its session; it loses it only if it was resuming one.
		if r, ok := w.live(f); ok && r.TerminalSession != "" && f.term.SessionLive(f.ctx, r.TerminalSession) {
			f.term.exitWith(t, r.TerminalSession, engineterminal.Exit{
				Code: 1, Screen: "No conversation found with session ID\nError: no rollout found",
			})
		}
	}},
	{"delete", func(t *testing.T, f testFixture, w *sessionWalk) {
		t.Helper()
		if err := f.usecase.PurgeChat(f.ctx, w.chatID); err != nil {
			return
		}
		f.wait()
		// S8: nothing kept for the chat outlives it.
		rows, err := f.ledger.Read(f.ctx, w.chatID, 0, 0)
		require.NoError(t, err)
		assert.Empty(t, rows)
		w.chatID, _ = f.spawn(t, w.provider)
	}},
}

// checkSessionInvariants asserts S1 and S2 after every step of the walk.
func checkSessionInvariants(t *testing.T, f testFixture, w *sessionWalk, trail []string) {
	t.Helper()
	// S1: one runner per vendor session (one per chat is the live table's key).
	all := agentusecase.AllLiveRunners(f.usecase.own)
	holders := map[string]int{}
	for _, r := range all {
		if c := f.conversation(t, r.CurrentChatID, r.ProviderID); r.CurrentChatID != "" && c.SessionID != "" {
			holders[r.ProviderID+"/"+c.SessionID]++
		}
	}
	for session, n := range holders {
		assert.Equal(t, 1, n, "S1 session %s, after %v", session, trail)
	}
	// S2: a chat with no runner says why its last one ended.
	// S3: and has no turn left open — nothing is left that could close it.
	if _, ok := w.live(f); !ok {
		assert.NotEmpty(t, agentusecase.ChatExitReason(f.usecase.RunnerUsecase, w.chatID),
			"S2, after %v", trail)
		assert.False(t, f.busy(t, w.chatID), "S3, after %v", trail)
	}
	assertTurnsStartOnTheLiveRunner(t, f, w.chatID, trail)
}

// concurrentOps are the user's own intents, which really do race each other
// from different tabs and clients.
var concurrentOps = []string{"send", "stop", "switch", "resume", "crash"}

// endsRunner reports whether an intent may leave the chat with no runner.
func endsRunner(op string) bool { return op == "stop" || op == "crash" }

func sessionOpNamed(name string) sessionOp {
	for _, op := range sessionOps {
		if op.name == name {
			return op
		}
	}
	panic("no session op " + name)
}

// Two intents at once — a send racing a stop, a switch racing a resume — leave
// the same invariants standing. Run under -race.
func TestSessionInvariants_HoldWhenIntentsRace(t *testing.T) {
	for seed := uint64(1); seed <= 10; seed++ {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := newFixture(t)
			rng := rand.New(rand.NewPCG(seed, seed))
			w := &sessionWalk{provider: "claude"}
			var runnerID string
			w.chatID, runnerID = f.spawn(t, w.provider)
			f.announce(t, runnerID, "race-"+w.chatID[:8])
			var trail []string
			for range 20 {
				a := sessionOpNamed(concurrentOps[rng.IntN(len(concurrentOps))])
				b := sessionOpNamed(concurrentOps[rng.IntN(len(concurrentOps))])
				trail = append(trail, a.name+"|"+b.name)
				// Each goroutine reads its own copy: a switch flips the walk's provider.
				wa, wb := *w, *w
				wa.rival, wb.rival = b.name, a.name
				var both sync.WaitGroup
				both.Add(2)
				go func() { defer both.Done(); a.do(t, f, &wa) }()
				go func() { defer both.Done(); b.do(t, f, &wb) }()
				both.Wait()
				f.wait()
				if live, ok := w.live(f); ok {
					w.provider = live.ProviderID
				}
				checkSessionInvariants(t, f, w, trail)
				if t.Failed() {
					return
				}
			}
			endWalk(t, f, w, trail)
		})
	}
}

// The send|stop interleaving the race walk hits: the send holds the chat's
// lock until it has delivered, so a racing Stop takes the chat only after
// it — and ends the runner the send was delivered to, saying why.
func TestSessionInvariants_AStopAfterADeliveredSendEndsItsRunnerAndSaysWhy(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-send-stop")

	sub, err := f.usecase.SubmitPrompt(f.ctx, chatID, "step", uuid.NewString(), "", nil)
	require.NoError(t, err)
	delivered, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, sub.RunnerID, delivered.ID, "the send names the runner it was delivered to")

	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()

	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
	assert.Equal(t, domain.AgentExitStopped, agentusecase.ChatExitReason(f.usecase.RunnerUsecase, chatID))
	assert.False(t, f.busy(t, chatID))
}

// The session invariants hold under any interleaving of what users, CLIs and
// vendor stores do (sessions plan §3), walked from fixed seeds so a failure
// replays exactly.
func TestSessionInvariants_HoldUnderRandomInterleavings(t *testing.T) {
	for seed := uint64(1); seed <= 10; seed++ {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			f := newFixture(t)
			rng := rand.New(rand.NewPCG(seed, seed))
			w := &sessionWalk{provider: "claude"}
			w.chatID, _ = f.spawn(t, w.provider)
			var trail []string
			for range 40 {
				op := sessionOps[rng.IntN(len(sessionOps))]
				trail = append(trail, op.name)
				op.do(t, f, w)
				f.wait()
				checkSessionInvariants(t, f, w, trail)
				if t.Failed() {
					return
				}
			}
		})
	}
}

// A chat closed mid-first-turn resumes the session Crowbar assigned at launch: the
// turn started, so the CLI wrote it, though no session_start ever landed.
func TestRegression_ResumeChat_ClosedMidFirstTurn_ResumesTheAssignedSession(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	assigned := argAfter(t, f.term.calls[0].argv, "--session-id")
	hook(t, f, runnerID, "claude", "user_prompt", map[string]any{"prompt": "first words", "session_id": assigned})

	require.NoError(t, f.usecase.KillChat(f.ctx, chatID))
	f.wait()
	_, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()

	require.Equal(t, 2, f.term.callCount())
	argv := f.term.calls[1].argv
	assert.Equal(t, assigned, argAfter(t, argv, "--resume"),
		"the session Crowbar assigned must be resumed; argv was %v", argv)
	assert.Equal(t, -1, slices.Index(argv, "--session-id"), "a resume must not also assign a fresh id")
}

// A long chat switched twice: the provider joining fresh is handed the chat's
// first ask, then an omission note, then the newest turns, in that order; the
// one resumed is handed its gap's newest turns behind the same note.
func TestRegression_LongChatHandoffKeepsTheFirstAskAcrossTwoSwitches(t *testing.T) {
	f := newFixture(t)
	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude-native")
	prompt(t, f, claudeRunner, "claude", "FIRST ASK: build the parser")
	for i := 0; i < 30; i++ {
		exchange(t, f, claudeRunner, "claude", fmt.Sprintf("claude turn %d", i))
	}

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	doc := configValue(t, f.term.calls[f.term.callCount()-1].argv, "developer_instructions=")
	assertHandoffOrder(t, doc, "FIRST ASK: build the parser", "earlier turns are omitted", "claude turn 29")

	f.announce(t, codexRunner, "sid-codex-native")
	for i := 0; i < 30; i++ {
		prompt(t, f, codexRunner, "codex", "go on")
		codexTurnStop(t, f, codexRunner, "sid-codex-native", fmt.Sprintf("codex turn %d", i))
	}
	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	argv := f.term.calls[f.term.callCount()-1].argv
	gap := argv[len(argv)-1]
	assertHandoffOrder(t, gap, "earlier turns are omitted", "codex turn 29")
	assert.NotContains(t, gap, "claude turn 29", "a resumed provider is not re-fed its own turns")
}

// assertHandoffOrder requires each part in doc, each after the one before.
func assertHandoffOrder(t *testing.T, doc string, parts ...string) {
	t.Helper()
	at := -1
	for _, p := range parts {
		i := strings.Index(doc, p)
		require.Greater(t, i, at, "%q must appear after the part before it in:\n%s", p, doc)
		at = i
	}
}

func TestSwitchProvider_UnknownTargetProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	spawnCount := f.term.callCount()
	terminatedCount := len(f.term.terminatedIDs())

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
	assert.Equal(t, spawnCount, f.term.callCount(), "an invalid target must not spawn anything")
	assert.Len(t, f.term.terminatedIDs(), terminatedCount,
		"target planning must fail before the outgoing TUI is touched")
	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
}
