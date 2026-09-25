package turn

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// onAPI is a delivery pumped off the runner's own api connection — the SAME
// marker pumpAPIConn carries in production (ingest.go's channelFor), and the
// only channel judged against the originated set.
func onAPI() context.Context { return inflight.WithAPITransport(context.Background()) }

// originatingRunners is a runner holding a live api connection that produced
// the named conversations, and nothing else — every other Runners method embeds
// the port and panics if reached, which is deliberate: the ownership guard must
// reach its api verdict from these two reads alone.
type originatingRunners struct {
	Runners
	originated map[string]bool
}

func (originatingRunners) ConfirmLaunch(string) {}

func (originatingRunners) HasLiveAPIConnection(string) bool { return true }

func (r originatingRunners) OriginatedSession(_, sessionID string) bool {
	return r.originated[sessionID]
}

// TestRegression_ASubagentsThreadIsForeignBeforeTheRunnerEverBindsASession is
// the identity half of the 2026-09-23 collab-agents bug on the HOOKS channel:
// an api-transport resume (thread/resume) announces no session at all, so the
// runner row carries only the conversation Crowbar LAUNCHED it on. With
// CurrentSession empty this guard had nothing to compare against and waved
// every child thread the provider pushed down the same connection straight
// into the user's transcript.
func TestRegression_ASubagentsThreadIsForeignBeforeTheRunnerEverBindsASession(t *testing.T) {
	t.Parallel()
	turns := &Turns{}
	runner := engineagents.Runner{LaunchSessionID: "thread-main"}

	assert.True(t, turns.namesAnotherConversation(context.Background(), runner,
		engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, SessionID: "thread-child"}),
		"the runner has no bound session, but it was launched on thread-main — a child thread is still not its conversation")
	assert.False(t, turns.namesAnotherConversation(context.Background(), runner,
		engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, SessionID: "thread-main"}),
		"its own launch conversation is never foreign")
}

// TestRegression_AChildThreadIsForeignWhenThisConnectionNeverOriginatedIt is
// the SECOND shape of that bug, live 2026-09-23 in chat d4912d6d: a FRESH
// api-transport chat has no launch session either — the thread id is minted
// inside the driver's own establish call, before pumpAPIConn starts — so the
// row fallback above was inert and every child thread was waved through again.
// The api channel is answered by CAUSE instead: our own driver opened the
// parent (apidriver.EstablishSession claims it), never the child.
func TestRegression_AChildThreadIsForeignWhenThisConnectionNeverOriginatedIt(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: originatingRunners{originated: map[string]bool{"thread-main": true}}}
	sessionless := engineagents.Runner{ID: "runner-1"}
	own := engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-main"}
	child := engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-child"}

	assert.False(t, turns.namesAnotherConversation(onAPI(), sessionless, own),
		"the conversation Crowbar's own driver minted is this chat's, whatever the row says")
	assert.True(t, turns.namesAnotherConversation(onAPI(), sessionless, child),
		"a thread nothing on this side ever opened is one the provider opened for itself")
}

// TestRegression_TheParentConversationIsNeverForeign is the exact failure the
// 2026-09-23 revert was needed for. The learned-baseline attempt latched onto
// whichever conversation spoke first, which could be a CHILD, and then read the
// PARENT's own frames as foreign — a codex chat running three subagents
// recorded zero subagents and zero wire tool calls, strictly worse than the
// leak it was closing. The originated set cannot do that: the parent is in it
// by construction, so no order of arrival can make it foreign.
func TestRegression_TheParentConversationIsNeverForeign(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: originatingRunners{originated: map[string]bool{"thread-parent": true}}}
	sessionless := engineagents.Runner{ID: "runner-1"}

	// The child speaks FIRST here, which is what defeated the learned baseline.
	assert.True(t, turns.namesAnotherConversation(onAPI(), sessionless,
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-child"}))
	assert.False(t, turns.namesAnotherConversation(onAPI(), sessionless,
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-parent"}),
		"the runner's own conversation must stay recordable no matter which thread spoke first")
}

// A bound conversation always wins over the launch one on the hooks channel: a
// /clear rebinds, and the conversation left behind must go back to being foreign.
func TestNamesAnotherConversation_ABoundSessionSupersedesTheLaunchOne(t *testing.T) {
	t.Parallel()
	turns := &Turns{}
	runner := engineagents.Runner{LaunchSessionID: "thread-main", CurrentSession: "thread-cleared"}

	assert.True(t, turns.namesAnotherConversation(context.Background(), runner,
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-main"}),
		"the launch conversation was superseded by a bind and is foreign again")
	assert.False(t, turns.namesAnotherConversation(context.Background(), runner,
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-cleared"}))
}

// TestRegression_AHooksDeliveryIsNeverJudgedAgainstTheOriginatedSet pins the
// companion-PTY hazard, and is why the hooks branch stays a row comparison.
// Every api-transport spawn also forks a hooks-wired companion PTY, which
// fires the descriptor's whole hook set under the CLI'S OWN session id — an id
// no driver of ours ever minted — and a provider's own internal sessions fire
// that same hook set under an id of their own (the recorded Stop.json is a
// codex memory-consolidation session's). Judged against the originated set,
// every real hook event of a codex chat would be dropped: a chat that goes
// permanently silent, strictly worse than the leak the guard exists to stop.
func TestRegression_AHooksDeliveryIsNeverJudgedAgainstTheOriginatedSet(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: originatingRunners{originated: map[string]bool{"thread-parent": true}}}
	// The row a pure api-transport spawn actually leaves behind: nothing bound,
	// nothing launched.
	sessionless := engineagents.Runner{ID: "runner-1"}
	companionPTY := engineagents.CanonicalEvent{
		Kind: engineagents.HookUserPrompt, SessionID: "cli-own-session",
	}

	assert.False(t, turns.namesAnotherConversation(context.Background(), sessionless, companionPTY),
		"a hooks delivery carries a different id namespace entirely; the originated set says nothing about it")
	assert.True(t, turns.namesAnotherConversation(onAPI(), sessionless, companionPTY),
		"the SAME id off the api connection is genuinely a conversation nothing here opened")
}

// TestNamesAnotherConversation_WithNoConnectionTheRowStandsIn pins the window
// the api branch deliberately does not judge: once the connection is gone its
// originated record is gone with it, and "originated nothing" then answers every
// id. A frame still draining out of a torn-down connection must fall back to the
// row comparison rather than be dropped wholesale.
func TestNamesAnotherConversation_WithNoConnectionTheRowStandsIn(t *testing.T) {
	t.Parallel()
	turns := &Turns{runners: fakeLiveConn{live: false}}
	bound := engineagents.Runner{ID: "runner-1", CurrentSession: "thread-main"}

	assert.False(t, turns.namesAnotherConversation(onAPI(), bound,
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-main"}),
		"the row still names the runner's own conversation, and nothing originated says otherwise")
	assert.True(t, turns.namesAnotherConversation(onAPI(), bound,
		engineagents.CanonicalEvent{Kind: engineagents.HookTurnStop, SessionID: "thread-child"}),
		"and a child thread stays foreign against it")
}
