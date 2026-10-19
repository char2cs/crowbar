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

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// Codex ends its own turn the instant it delegates to a subagent, so only the
// ledger's background-work clause keeps the chat busy while that subagent
// runs. Sampled against a real codex delegation, with no synthetic hooks.
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
		mu                   sync.Mutex
		samples              int
		sawOpenWait          bool
		sawIdleWhileWaitOpen bool
		lastWaitStatus       string
		stop                 = make(chan struct{})
		done                 = make(chan struct{})
	)
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			case <-time.After(300 * time.Millisecond):
			}
			snap, err := h.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
			if err != nil {
				continue
			}
			activity, err := h.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
			if err != nil {
				continue
			}
			openWait := false
			for _, c := range activity.ToolCalls {
				if !isSubagentWaitCall(c.Name) {
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
				if !snap.Ledger.Busy {
					sawIdleWhileWaitOpen = true
				}
			}
			mu.Unlock()
		}
	}()

	awaitTurnComplete(t, h, wsID, chatID, "codex")

	// awaitTurnComplete releases once the ledger reads idle: a wait call still
	// running then is the same defect, seen from the barrier.
	finalActivity, err := h.app.Usecases.AgentTurn.ReadActivity(context.Background(), chatID, 0, 0)
	require.NoError(t, err)
	for _, c := range finalActivity.ToolCalls {
		if isSubagentWaitCall(c.Name) {
			lastWaitStatus = c.Status
		}
	}

	close(stop)
	<-done

	mu.Lock()
	defer mu.Unlock()

	require.Positive(t, samples, "the poller must have sampled at least once")
	require.True(t, sawOpenWait,
		"never observed an open wait tool call — codex did not actually delegate a real subagent, "+
			"so this run could not exercise the race at all")
	assert.False(t, sawIdleWhileWaitOpen,
		"the ledger read idle while a real wait tool call was still running: the "+
			"spinner goes dark under a genuinely live subagent")
	assert.NotEqual(t, domain.ToolStatusRunning, lastWaitStatus,
		"awaitTurnComplete released while the wait call was still running: "+
			"the barrier itself agrees the turn is over before the subagent actually is")
}

// isSubagentWaitCall matches the delegation's wait call under either
// transport's name for it: "wait" over api, "…wait_agent" over hooks.
func isSubagentWaitCall(name string) bool {
	return strings.Contains(strings.ToLower(name), "wait")
}
