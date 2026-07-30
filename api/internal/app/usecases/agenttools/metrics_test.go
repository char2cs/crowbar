package agenttools_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMetrics_CountsCallsAndFailuresPerTool(t *testing.T) {
	m := agenttools.NewMetrics()
	m.Record("set_chat_title", true)
	m.Record("set_chat_title", true)
	m.Record("set_chat_title", false)
	m.Record("list_workspaces", true)

	snap := m.Snapshot()
	require.Equal(t, agenttools.ToolStat{Calls: 3, Failures: 1}, snap["set_chat_title"])
	require.Equal(t, agenttools.ToolStat{Calls: 1, Failures: 0}, snap["list_workspaces"])
}

// TestMetrics_IsSafeUnderConcurrentCalls drives Record from many goroutines at
// once — a sequential loop would pass even with no locking at all, so this
// must run under `go test -race` to mean anything.
func TestMetrics_IsSafeUnderConcurrentCalls(t *testing.T) {
	m := agenttools.NewMetrics()
	const goroutines = 50
	const perGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				m.Record("concurrent_tool", (id+i)%3 != 0)
			}
		}(g)
	}
	wg.Wait()

	wantFailures := 0
	for g := 0; g < goroutines; g++ {
		for i := 0; i < perGoroutine; i++ {
			if (g+i)%3 == 0 {
				wantFailures++
			}
		}
	}

	stat := m.Snapshot()["concurrent_tool"]
	require.Equal(t, goroutines*perGoroutine, stat.Calls)
	require.Equal(t, wantFailures, stat.Failures)
}

// TestMetrics_SnapshotReturnsACopy guards against a caller's mutation of the
// returned map reaching back into the live counters — Snapshot must hand back
// a copy, not the internal map itself.
func TestMetrics_SnapshotReturnsACopy(t *testing.T) {
	m := agenttools.NewMetrics()
	m.Record("set_chat_title", true)

	snap := m.Snapshot()
	snap["set_chat_title"] = agenttools.ToolStat{Calls: 999, Failures: 999}
	snap["injected"] = agenttools.ToolStat{Calls: 1}

	fresh := m.Snapshot()
	require.Equal(t, agenttools.ToolStat{Calls: 1, Failures: 0}, fresh["set_chat_title"])
	require.NotContains(t, fresh, "injected")
}

// TestMetrics_NilReceiverIsANoOp proves the fail-open contract Metrics itself
// must uphold: a nil *Metrics — the zero value of Deps.Metrics — must never
// panic, unlike every other port in Deps where nil means "not registered".
func TestMetrics_NilReceiverIsANoOp(t *testing.T) {
	var m *agenttools.Metrics
	require.NotPanics(t, func() { m.Record("set_chat_title", true) })
	require.Nil(t, m.Snapshot())
}

// TestToolSet_RecordsEveryCallIncludingUnauthorized proves the deliberate
// attribution choice documented on ToolSet.Call: Resolve fails before name is
// checked against the registered tools, and the call is still counted under
// the literal name the caller asked for — a rejected attempt at a specific
// tool is exactly the datum this counter exists to surface.
func TestToolSet_RecordsEveryCallIncludingUnauthorized(t *testing.T) {
	metrics := agenttools.NewMetrics()
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(minter,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})

	ts := agenttools.NewToolSet(agenttools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", "forged-token")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, agenttools.ErrUnauthorized)

	stat := metrics.Snapshot()["set_chat_title"]
	require.Equal(t, 1, stat.Calls)
	require.Equal(t, 1, stat.Failures)
}

// TestToolSet_RecordsSuccessfulCalls is the counterpart to the unauthorized
// case above: a call that actually reaches its handler and succeeds must be
// counted as a call with zero failures, not merely as a non-error return.
func TestToolSet_RecordsSuccessfulCalls(t *testing.T) {
	metrics := agenttools.NewMetrics()
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(minter,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := minter.Mint("RUN")

	ts := agenttools.NewToolSet(agenttools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", tok)

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Refactor auth"}`))
	require.NoError(t, err)

	stat := metrics.Snapshot()["set_chat_title"]
	require.Equal(t, 1, stat.Calls)
	require.Equal(t, 0, stat.Failures)
}

// TestToolSet_NilMetricsStillRegistersAllEightTools is the fail-open guard:
// every other Deps port suppresses its tool group when nil, but Metrics must
// not — losing the call counters is never a reason to lose a capability. The
// shared toolsetOn fixture never sets Metrics, so this also doubles as proof
// that a production Deps left without Metrics wired keeps working.
func TestToolSet_NilMetricsStillRegistersAllEightTools(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.Len(t, ts.Tools(), 8)

	out, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.NoError(t, err)
	require.Contains(t, out, "x")
}
