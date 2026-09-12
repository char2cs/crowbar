//go:build integration

package agent_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestRegression_CodexBackgroundedSubagentKeepsChatWorking guards a live-
// reported bug: codex ends its own top-level turn the instant it delegates to
// a subagent (its "wait" tool call is what tracks the delegation, not an open
// turn — see turn.go's own doc on restateAsyncWork/fallbackAsyncWork), so
// nothing but Crowbar's OWN open-tool-call fallback keeps chat.Working true
// while that subagent is still running. If that fallback ever misses the open
// "wait" call, the spinner goes dark under a live subagent and the composer
// reads idle — reported live, repeatedly, against this exact repo.
//
// This drives a REAL codex process through a REAL subagent delegation (no
// synthetic hooks, no mocked activity) and polls chat.Working and the
// activity ledger's own "wait" tool call concurrently for the whole window,
// so a disagreement between "a wait call is Status==Running" and
// "chat.Working==false" is caught the instant it happens rather than
// inferred after the fact.
func TestRegression_CodexBackgroundedSubagentKeepsChatWorking(t *testing.T) {
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-subagent-working", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "codex")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)

	drive(t, h, tap, termSessID,
		"Use your Agent/Task subagent delegation tool right now to run one real subagent. "+
			"Its job: run the shell command `sleep 20 && echo done-in-subagent` and report back "+
			"exactly what it printed. Actually invoke a real subagent for this — do not simulate, "+
			"describe, or fabricate it. Say as little as possible while you wait for it to finish.")

	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID, "codex never bound a session: %+v", runner)

	var (
		mu                           sync.Mutex
		samples                      int
		sawOpenWait                  bool
		sawWorkingFalseWhileWaitOpen bool
		lastWaitStatus               string
		stop                         = make(chan struct{})
		done                         = make(chan struct{})
	)
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(300 * time.Millisecond):
			}
			chat, err := h.app.Usecases.AgentChat.GetChat(context.Background(), chatID)
			if err != nil {
				continue
			}
			activity, err := h.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
			if err != nil {
				continue
			}
			openWait := false
			for _, c := range activity.ToolCalls {
				if c.Name != "wait" {
					continue
				}
				if c.Status == domain.ToolStatusRunning {
					openWait = true
				}
			}
			mu.Lock()
			samples++
			if openWait {
				sawOpenWait = true
				if !chat.Working {
					sawWorkingFalseWhileWaitOpen = true
				}
			}
			mu.Unlock()
		}
	}()

	awaitTurnComplete(t, h, wsID, chatID, "codex")

	// awaitTurnComplete releases the instant chat.Working reads false. If a
	// "wait" call is STILL Status==Running right then, the barrier itself
	// released early — the same defect from the reader's side, independent
	// of the poller above.
	finalActivity, err := h.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
	require.NoError(t, err)
	for _, c := range finalActivity.ToolCalls {
		if c.Name == "wait" {
			lastWaitStatus = c.Status
		}
	}

	close(stop)
	<-done

	mu.Lock()
	defer mu.Unlock()

	require.Positive(t, samples, "the poller must have sampled at least once")
	require.True(t, sawOpenWait,
		"never observed an open 'wait' tool call — codex did not actually delegate a real subagent, "+
			"so this run could not exercise the race at all")
	assert.False(t, sawWorkingFalseWhileWaitOpen,
		"chat.Working read false while a real 'wait' tool call was still Status==Running: the "+
			"spinner goes dark under a genuinely live subagent")
	assert.NotEqual(t, domain.ToolStatusRunning, lastWaitStatus,
		"awaitTurnComplete (chat.Working==false) released while the 'wait' call was still running: "+
			"the barrier itself agrees the turn is over before the subagent actually is")
}
