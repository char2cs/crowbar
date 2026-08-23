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
