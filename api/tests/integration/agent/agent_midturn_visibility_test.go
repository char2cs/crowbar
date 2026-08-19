//go:build integration

package agent_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestRegression_AMessageSaidMidTurnIsVISIBLEBeforeTheTurnEnds is the half of the
// mid-turn transcript defect that TestAgent_LiveClaudeRecordsEveryMessageOfATurn
// cannot see, and the half the user kept reporting after that one went green.
//
// That test reads the ledger only after awaitTurnComplete. So it proves the
// earlier message is EVENTUALLY RECORDED, in order and exactly once — and it
// passes just as happily if BOTH messages are drained at turn_stop, which is
// indistinguishable from the bug: "Claude might say something, then call a tool,
// and then say something else, and the message appears ONLY when it finished
// working."
//
// Recording and visibility are different claims. Crowbar reads the provider's own
// transcript at tool_pre (recordSaidSoFar) precisely so a message said before slow
// work lands in the ledger WHILE the work is still running. Whether claude has
// actually flushed that line to its JSONL by then is a fact about claude's write
// timing that no fixture can answer — the same reason its sibling is a live test.
//
// So this asserts the timing directly: the first marker must be readable from the
// ledger while the chat is STILL working. The poller runs against the same
// ReadMessages the chat pane polls, and records the working flag at the moment it
// first sees the marker, so a failure says which of the two states it arrived in.
func TestRegression_AMessageSaidMidTurnIsVISIBLEBeforeTheTurnEnds(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-midturn", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID, "claude never bound a session: %+v", runner)

	const (
		first  = "MARKER-ONE-6F2A"
		second = "MARKER-TWO-"
		secret = "SECRET-4B7C"
	)

	// The poller is started BEFORE the prompt so it cannot miss an early flush, and
	// it samples the two facts together: is the marker readable, and is the chat
	// still working. Sampling them in one pass is what makes "visible while
	// working" a single observation rather than two that could straddle the edge.
	var (
		mu          sync.Mutex
		seenWorking bool
		seenAtAll   bool
		firstSeenAt time.Duration
		samples     int
		stop        = make(chan struct{})
		done        = make(chan struct{})
		startedAt   = time.Now()
	)
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(250 * time.Millisecond):
			}
			replies := assistantReplies(readLedgerTurns(t, h, wsID, chatID), "claude")
			visible := false
			for _, r := range replies {
				if strings.Contains(r, first) {
					visible = true
					break
				}
			}
			chat, err := h.app.Usecases.Agent.GetChat(context.Background(), chatID)
			if err != nil {
				continue
			}
			mu.Lock()
			samples++
			if visible && !seenAtAll {
				seenAtAll = true
				firstSeenAt = time.Since(startedAt)
				seenWorking = chat.Working
			}
			mu.Unlock()
		}
	}()

	drive(t, h, tap, termSessID,
		"You are being driven by an automated harness. Follow this script exactly, in order. "+
			"(1) Your first message must be exactly this line and nothing else: "+first+" "+
			"(2) Then call the Bash tool with exactly this command: sleep 25 && echo "+secret+" "+
			"(3) Your final message must be exactly the line "+second+
			" immediately followed by the word that command printed. "+
			"Do not skip the Bash call and do not answer step 3 from memory.")

	awaitTurnComplete(t, h, wsID, chatID, "claude")
	close(stop)
	<-done

	mu.Lock()
	defer mu.Unlock()

	// Precondition: the script was actually followed, so a failure below is about
	// visibility rather than about a model that skipped a step.
	replies := assistantReplies(readLedgerTurns(t, h, wsID, chatID), "claude")
	require.Equal(t, 1, countContaining(replies, first),
		"precondition: the mid-turn message must be recorded exactly once (got %d replies: %q)",
		len(replies), replies)
	require.Equal(t, 1, countContaining(replies, second+secret),
		"precondition: claude must have run the Bash call and quoted its output")
	require.Positive(t, samples, "the poller must have sampled the ledger at least once")

	require.True(t, seenAtAll,
		"the poller never saw the mid-turn message at all before the turn ended, across %d samples",
		samples)
	assert.True(t, seenWorking,
		"the message claude said BEFORE its slow work became readable only after the chat "+
			"stopped working (first seen %s after the prompt, with working=false). Recording it "+
			"at turn_stop is the defect: the user watches a spinner with nothing under it for the "+
			"whole tool call, then every message lands at once.", firstSeenAt)
	t.Logf("mid-turn message first visible %s after the prompt, working=%v, %d samples",
		firstSeenAt, seenWorking, samples)
}
