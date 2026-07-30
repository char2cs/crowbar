//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRegression_ChatStoppedMidTurn_SpunTheWorkspaceForever
//
// THE BUG. A turn that opened moments before its vendor CLI was taken off the chat was
// never closed by anything, so the chat — and the workspace's whole working overlay with
// it — spun FOREVER. Nothing retried, and no later event cleared it: only the user
// resuming that chat and completing another turn ever did.
//
// The cause was WHERE the teardown asked "is there a turn here to close?". It asked the
// READ MODEL (agent.closeAbandonedTurn read domain.AgentChat.Working through GetChat) and
// went home when the answer was no — but the read model is folded by an ASYNCHRONOUS
// projection, while the turn is durable in the event log the instant the user_prompt hook
// returns. A prompt landing just before a teardown therefore read back as "idle", and the
// turn was left open with nobody left to close it: the CLI the teardown had just killed
// will never send the turn_stop.
//
// WHAT THIS TEST IS, AND IS NOT. It does not reproduce the losing interleaving, and it
// cannot. An earlier draft went further than this one to try: it fired the stop with the
// prompt's projection deliberately un-quiesced, and measured against the broken code, 120
// rounds of THAT shape — with and without CPU saturation — never failed once, because even
// then the HTTP request boundary gives the projection time to fold. The lost race lives in
// a window microseconds wide, so pinning it needs an injection point INSIDE the teardown,
// which is where the deterministic guards for it are: TestSwitchProvider_MidTurn_Closes-
// TheOutgoingTurn and TestRegression_AbortedSwitchMidTurn_DoesNotLeaveTheChatSpinning-
// Forever deliver the prompt from inside TerminateGraceful, and commands.
// TestRegression_AbandonTurn_* pins the rule that replaced the racy guard.
//
// What this DOES gate is the end-to-end invariant those unit tests are about, over the
// public surface, on a teardown door none of them uses: STOP. A stop is the one door that
// tears down MID-TURN by design (a provider switch waits for the in-flight turn first), so
// it is the path where a chat's last turn is most often orphaned — and after the fix its
// correctness rests entirely on the command refusing or accepting the abandon for itself.
// Nothing in this suite covered it before.
//
// It asserts the WORKSPACE overlay rather than the chat, because that is the wedge the
// user reported: one chat with an orphaned turn lights the spinner on the whole workspace.
// Both REST read paths are checked, so a refetch cannot disagree with the live frame.
//
// The Quiesce after the prompt is there to make the working=true line below a real
// PRECONDITION rather than a hopeful one — and it is a second, stronger reason this test
// cannot reproduce the loss: it settles the very projection whose lag was the bug, before
// the stop is ever issued. Both reasons are stated because neither alone would be honest.
//
// No timing anywhere: every step blocks on a real signal (the HTTP response, then
// Quiesce).
func TestRegression_ChatStoppedMidTurn_SpunTheWorkspaceForever(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	chatID, runnerID := createLiveStubChat(t, h, imported)

	// The prompt opens the turn.
	postAgentHook(t, h, imported, runnerID, "user_prompt", `{"prompt":"a long-running task"}`)
	h.Quiesce()
	requireRESTWorking(t, h, imported, true)

	// The user closes the tab while the agent is still answering. The CLI is SIGTERMed
	// mid-turn, so the turn_stop that would have closed this turn is never coming — the
	// teardown is the only thing that can, and it gets no second chance.
	resp := h.raw(http.MethodPost, wsBase(imported)+"/agent/chats/"+chatID+"/stop", nil, http.StatusAccepted)
	_ = resp.Body.Close()
	h.Quiesce()

	require.Empty(t, getAgentChat(t, h, wsBase(imported), chatID).LiveRunnerID,
		"precondition: the stop took the CLI off the chat, so nothing but the teardown can ever close its turn")
	requireRESTWorking(t, h, imported, false)
}
