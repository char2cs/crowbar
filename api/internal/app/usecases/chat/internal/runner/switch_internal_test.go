package runner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubRunnerStoreForResumable answers only ConversationsForChat — the one call
// resumableConversation makes on the runner store. The embedded nil interface
// panics on anything else, so a test relying on a second method fails loudly
// instead of silently zero-valuing.
type stubRunnerStoreForResumable struct {
	agentrunner.EventStore
	convs []engineagents.ChatConversation
}

func (s stubRunnerStoreForResumable) ConversationsForChat(
	context.Context, string,
) ([]engineagents.ChatConversation, error) {
	return s.convs, nil
}

// TestRegression_ResumableConversation_OldConversationWithNoRecordedTurns_ResumesAnyway
// is the fix for the bug that bricked every pre-migration chat in production: PR #151
// introduced agent_turns (queried here via activity.LastTurnForSession) to record turns
// from hooks, but that table has zero rows for any conversation that happened before it
// existed — which looks IDENTICAL to a provider that announced a session and crashed
// before its first turn (the guard's original, legitimate target). Every old chat's real,
// resumable session id was being thrown away and a blank one spawned in its place.
//
// The fix distinguishes the two by age: a conversation first seen long ago (like this
// one, weeks old) is trusted and resumed anyway, falling back to chat.LastActivityAt as
// the gap cutoff since there is no per-turn record to be more precise with.
func TestRegression_ResumableConversation_OldConversationWithNoRecordedTurns_ResumesAnyway(t *testing.T) {
	weeksAgo := time.Now().Add(-21 * 24 * time.Hour)
	lastActivity := time.Now().Add(-2 * time.Hour)

	rs := &Runners{
		runnerStore: stubRunnerStoreForResumable{convs: []engineagents.ChatConversation{
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-legacy-session", FirstSeenAt: weeksAgo},
		}},
		activity: stubActivityForAttach{found: false},
	}
	chat := domain.Chat{ID: "chat-1", LastActivityAt: lastActivity}

	sessionID, leftAt, err := rs.resumableConversation(context.Background(), chat, "claude")

	require.NoError(t, err)
	assert.Equal(t, "sid-legacy-session", sessionID,
		"the real, pre-migration session id must be trusted and resumed, not thrown away")
	assert.True(t, leftAt.Equal(lastActivity),
		"with no per-turn record to draw the gap from, chat.LastActivityAt must stand in for it; got %v want %v",
		leftAt, lastActivity)
}

// TestRegression_ResumableConversation_RecentConversationWithNoRecordedTurns_StillSpawnsFresh
// is the regression-of-the-regression-guard: the fix above must not also swallow the
// genuine race it was carved out of. A provider that announces a session and crashes
// before completing its first turn looks — in the activity table — identical to an old
// conversation: zero rows either way. Only recency tells them apart, and a session
// announced moments ago with nothing recorded is still that crash, not an old
// conversation, and must still be refused so a fresh one is spawned instead.
func TestRegression_ResumableConversation_RecentConversationWithNoRecordedTurns_StillSpawnsFresh(t *testing.T) {
	justNow := time.Now().Add(-5 * time.Second)

	rs := &Runners{
		runnerStore: stubRunnerStoreForResumable{convs: []engineagents.ChatConversation{
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-crashed-session", FirstSeenAt: justNow},
		}},
		activity: stubActivityForAttach{found: false},
	}
	chat := domain.Chat{ID: "chat-1", LastActivityAt: time.Now().Add(-30 * 24 * time.Hour)}

	sessionID, leftAt, err := rs.resumableConversation(context.Background(), chat, "claude")

	require.NoError(t, err)
	assert.Empty(t, sessionID, "a session announced moments ago with no recorded turn is still the crash race, not an old conversation")
	assert.True(t, leftAt.IsZero(), "a refused resume carries no gap cutoff")
}

// TestRegression_ResumableConversation_TurnlessSessionButChatHasOtherTurns_AlwaysSpawnsFresh
// is the fix for a live bug: a session that crashes before its first turn and is not
// retried within sessionAnnounceCrashWindow ages past the crash-race check and falls
// into the "predates the activity table" branch forever after — every later resume
// attempt re-resumes the same turnless session, finds nothing to attach to, and the CLI
// exits again in a second or two, looping "This agent has exited" indefinitely with no
// escape. Age alone cannot tell a crash apart from legacy data once it exceeds the
// window, but this SAME PROVIDER's other recorded turns on this chat can: if it has ever
// produced a real turn (under a different session), the activity table was plainly live
// for this (chat, provider) pair, so a turnless session on it — however old — cannot be
// pre-migration data and must never be resumed.
func TestRegression_ResumableConversation_TurnlessSessionButChatHasOtherTurns_AlwaysSpawnsFresh(t *testing.T) {
	weeksAgo := time.Now().Add(-21 * 24 * time.Hour)
	hoursAgo := time.Now().Add(-2 * time.Hour)

	rs := &Runners{
		runnerStore: stubRunnerStoreForResumable{convs: []engineagents.ChatConversation{
			// Oldest first, per ConversationsForChat's own contract — an earlier
			// claude session that DID record a turn, then a later one that crashed
			// before recording anything.
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-earlier-claude-session", FirstSeenAt: weeksAgo},
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-crashed-session", FirstSeenAt: hoursAgo},
		}},
		activity: stubActivityBySession{wantSession: "sid-earlier-claude-session"},
	}
	chat := domain.Chat{ID: "chat-1", LastActivityAt: hoursAgo}

	sessionID, leftAt, err := rs.resumableConversation(context.Background(), chat, "claude")

	require.NoError(t, err)
	assert.Empty(t, sessionID,
		"an earlier session of this SAME provider having a recorded turn proves the activity table was live for it; the current turnless session must never be trusted as legacy data, however old")
	assert.True(t, leftAt.IsZero(), "a refused resume carries no gap cutoff")
}

// TestRegression_ResumableConversation_OtherProviderHasTurns_DoesNotBlockThisProvidersLegacyResume
// is the fix for the fix above: the sibling-session check must be scoped to the provider
// actually being resumed, not the whole chat. A chat that switched providers has real,
// table-live history for the OTHER provider — that says nothing about whether THIS
// provider's own, genuinely pre-migration session predates the table, and must not stop
// it from being trusted and resumed.
func TestRegression_ResumableConversation_OtherProviderHasTurns_DoesNotBlockThisProvidersLegacyResume(t *testing.T) {
	weeksAgo := time.Now().Add(-21 * 24 * time.Hour)
	lastActivity := time.Now().Add(-2 * time.Hour)

	rs := &Runners{
		runnerStore: stubRunnerStoreForResumable{convs: []engineagents.ChatConversation{
			{ChatID: "chat-1", ProviderID: "claude", SessionID: "sid-legacy-claude-session", FirstSeenAt: weeksAgo},
			{ChatID: "chat-1", ProviderID: "codex", SessionID: "sid-codex-with-turns", FirstSeenAt: weeksAgo},
		}},
		// Only codex's session has a recorded turn — claude's own is genuinely
		// turnless, pre-migration data.
		activity: stubActivityBySession{wantSession: "sid-codex-with-turns"},
	}
	chat := domain.Chat{ID: "chat-1", LastActivityAt: lastActivity}

	sessionID, leftAt, err := rs.resumableConversation(context.Background(), chat, "claude")

	require.NoError(t, err)
	assert.Equal(t, "sid-legacy-claude-session", sessionID,
		"codex's own recorded turns say nothing about claude's session on the same chat; it must still be trusted and resumed")
	assert.True(t, leftAt.Equal(lastActivity),
		"with no per-turn record to draw the gap from, chat.LastActivityAt must stand in for it; got %v want %v",
		leftAt, lastActivity)
}

// stubRunnerStoreLiveness answers only LiveRunnerForChat, either with a runner or
// with ErrNotFound — the two answers displaceForSwitch's work check now turns on.
type stubRunnerStoreLiveness struct {
	agentrunner.EventStore
	runner engineagents.Runner
	live   bool
}

func (s stubRunnerStoreLiveness) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	if !s.live {
		return engineagents.Runner{}, agentrunner.ErrNotFound
	}
	return s.runner, nil
}

// runnersForDisplace builds the smallest Runners that can run displaceForSwitch:
// a real turn-start gate and in-flight registry, an empty on-disk prompt journal
// (so the delivery guard passes), and the two answers under test.
func runnersForDisplace(t *testing.T, working, live bool) *Runners {
	t.Helper()
	return &Runners{
		ws:            fakeWSReader{chatsDir: t.TempDir()},
		prompts:       agentjournal.NewPromptRequests(),
		turnStarts:    inflight.NewGate(),
		inflightTurns: inflight.NewTurns(),
		turns:         stubTurnsForAttach{working: working},
		runnerStore: stubRunnerStoreLiveness{
			live:   live,
			runner: engineagents.Runner{ID: "r-1"},
		},
	}
}

// TestRegression_DisplaceForSwitch_DormantChatFlaggedWorking_DoesNotWaitForever is
// the fix for a chat that could only be abandoned.
//
// switchProviderLocked's retry loop asks displaceForSwitch whether to go round
// again, and a `working` chat answered yes so the outgoing TUI is kept alive until
// a later hook restates the work level as zero. On a DORMANT chat that wait can
// never end: there is no CLI to finish the work and none to send the hook. The loop
// therefore spun forever — about one lap per awaitTurnOrForce deadline — holding
// the chat's spawn gate, which is a plain mutex with no context on it. Every later
// resume, prompt and switch on that chat queued behind it and never answered at
// all, with no access-log line either, because the log is written on completion.
//
// ResumeChat enters this same locked path, so the one call whose job is to bring a
// dormant chat back was the call the stale flag stranded: the client sat on its
// "Resuming this chat…" spinner — which carries no button — until the user gave up
// on the chat. A durable `working` outliving its CLI is a REACHABLE state, not a
// hypothetical: a SIGKILL mid-background-work sends no final stop, and boot
// reconciliation only heals chats still reachable from a live runner row.
func TestRegression_DisplaceForSwitch_DormantChatFlaggedWorking_DoesNotWaitForever(t *testing.T) {
	rs := runnersForDisplace(t, true /* working */, false /* live */)

	retry, err := rs.displaceForSwitch(context.Background(), domain.Chat{ID: "chat-1"})

	require.NoError(t, err)
	assert.False(t, retry,
		"a dormant chat's stale working flag must not ask the caller to loop again: "+
			"nothing can ever clear it, so the retry holds the chat's spawn gate forever "+
			"and every later resume on that chat blocks with no response at all")
}

// TestRegression_DisplaceForSwitch_LiveRunnerFlaggedWorking_StillWaits is the
// regression-of-the-regression-guard. The fix above must not also swallow the real
// case the retry exists for: a turn_stop that handed work to the background after
// the first await released its runner-scoped turn. There the CLI is alive, it is
// genuinely still working, and quitting it there costs the answer — so a LIVE
// runner must still be waited for exactly as before.
func TestRegression_DisplaceForSwitch_LiveRunnerFlaggedWorking_StillWaits(t *testing.T) {
	rs := runnersForDisplace(t, true /* working */, true /* live */)

	retry, err := rs.displaceForSwitch(context.Background(), domain.Chat{ID: "chat-1"})

	require.NoError(t, err)
	assert.True(t, retry,
		"a LIVE runner still doing background work must still be waited for — "+
			"there is a CLI to finish it and a hook coming to say so")
}

// TestRegression_QuitOutgoingCLI_TerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY
// mirrors TestRegression_RetireTerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY
// (lifecycle_internal_test.go): quitOutgoingCLI — SwitchProvider's own teardown of the
// chat's outgoing CLI, reached from displaceForSwitch — had the SAME gap retire() did
// before its fix. It terminated only live.TerminalSession, the ORIGINAL companion PTY
// every api-transport spawn forks alongside its connection and never reassigns; once
// SwitchToTerminal has run, that field names a different, already-abandoned process, and
// the actually-attached native-view PTY it forks separately (tracked only in rs.attached)
// kept running indefinitely — a real process leak reachable by switching provider on a
// chat while it is attached to its native terminal view, with rs.attached left answering
// AttachedTerminalSession for a runner id the switch has already moved past.
func TestRegression_QuitOutgoingCLI_TerminatesTheAttachedNativeViewNotJustTheStaleCompanionPTY(t *testing.T) {
	term := &fakeTermForAttach{}
	store := &stopRetireRunnerStore{
		runner: engineagents.Runner{ID: "runner-1", TerminalSession: "companion-pty-term"},
	}
	rs := &Runners{
		runnerStore:   store,
		attached:      newAttachRegistry(),
		apiConns:      newAPIConnRegistry(),
		term:          term,
		inflightTurns: inflight.NewTurns(),
	}
	rs.attached.set("runner-1", attachedView{termSessID: "native-view-term"})

	require.NoError(t, rs.quitOutgoingCLI(context.Background(), "chat-1"))

	assert.Contains(t, term.terminated, "companion-pty-term",
		"the original companion PTY is still a real process and must still be torn down")
	assert.Contains(t, term.terminated, "native-view-term",
		"the actually-attached native view PTY — the one the user was looking at — must be torn down too")
	_, stillAttached := rs.attached.get("runner-1")
	assert.False(t, stillAttached, "the attach registry entry must be forgotten, or it answers for a runner nothing will revisit")
	assert.True(t, store.displaced, "the outgoing runner must still be displaced from the chat")
}
