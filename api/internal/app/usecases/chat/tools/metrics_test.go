package tools_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMetrics_CountsCallsAndFailuresPerTool(t *testing.T) {
	m := tools.NewMetrics()
	m.Record("set_chat_title", true)
	m.Record("set_chat_title", true)
	m.Record("set_chat_title", false)
	m.Record("list_workspaces", true)

	snap := m.Snapshot()
	require.Equal(t, tools.ToolStat{Calls: 3, Failures: 1}, snap["set_chat_title"])
	require.Equal(t, tools.ToolStat{Calls: 1, Failures: 0}, snap["list_workspaces"])
}

// TestMetrics_IsSafeUnderConcurrentCalls drives Record from many goroutines at
// once — a sequential loop would pass even with no locking at all, so this
// must run under `go test -race` to mean anything.
func TestMetrics_IsSafeUnderConcurrentCalls(t *testing.T) {
	m := tools.NewMetrics()
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
	m := tools.NewMetrics()
	m.Record("set_chat_title", true)

	snap := m.Snapshot()
	snap["set_chat_title"] = tools.ToolStat{Calls: 999, Failures: 999}
	snap["injected"] = tools.ToolStat{Calls: 1}

	fresh := m.Snapshot()
	require.Equal(t, tools.ToolStat{Calls: 1, Failures: 0}, fresh["set_chat_title"])
	require.NotContains(t, fresh, "injected")
}

// TestMetrics_NilReceiverIsANoOp proves the fail-open contract Metrics itself
// must uphold: a nil *Metrics — the zero value of Deps.Metrics — must never
// panic, unlike every other port in Deps where nil means "not registered".
func TestMetrics_NilReceiverIsANoOp(t *testing.T) {
	var m *tools.Metrics
	require.NotPanics(t, func() { m.Record("set_chat_title", true) })
	require.Nil(t, m.Snapshot())
}

// TestToolSet_RecordsEveryCallIncludingUnauthorized proves the deliberate
// attribution choice documented on ToolSet.Call: Resolve fails before name is
// checked against the registered tools, and the call is still counted under
// the literal name the caller asked for — a rejected attempt at a specific
// tool is exactly the datum this counter exists to surface.
func TestToolSet_RecordsEveryCallIncludingUnauthorized(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})

	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", "forged-token")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, tools.ErrUnauthorized)

	stat := metrics.Snapshot()["set_chat_title"]
	require.Equal(t, 1, stat.Calls)
	require.Equal(t, 1, stat.Failures)
}

// TestToolSet_RecordsSuccessfulCalls is the counterpart to the unauthorized
// case above: a call that actually reaches its handler and succeeds must be
// counted as a call with zero failures, not merely as a non-error return.
func TestToolSet_RecordsSuccessfulCalls(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := minter.Mint("RUN")

	ts := tools.NewToolSet(tools.Deps{
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

// TestToolSet_FoldsUnknownToolNamesIntoOneBucket bounds the counter map. The
// name reaches Record straight off the wire, before it is checked against the
// registered tools, and the caller is a MODEL — so a hallucinating agent could
// otherwise add one map entry per invented name and grow it for the whole life
// of the daemon. Every unregistered name lands in a single bucket; the real
// tools keep their own attribution.
func TestToolSet_FoldsUnknownToolNamesIntoOneBucket(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", minter.Mint("RUN"))

	for _, invented := range []string{"rm_rf", "list_secrets", "get_review_scop", "🙂"} {
		_, err := ts.Call(context.Background(), invented, json.RawMessage(`{}`))
		require.Error(t, err)
	}
	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.NoError(t, err)

	snap := metrics.Snapshot()
	require.Equal(t, tools.ToolStat{Calls: 4, Failures: 4}, snap[tools.UnknownToolMetric])
	require.Equal(t, tools.ToolStat{Calls: 1, Failures: 0}, snap["set_chat_title"])
	require.Len(t, snap, 2, "four invented names must not add four keys to the counter map")
}

// A name is folded by whether THIS ToolSet registered it, not by a hardcoded
// list: a tool suppressed because its dependency is missing is genuinely not a
// tool this caller has, so an attempt at it is not something to attribute
// per-name either.
func TestToolSet_FoldsAToolThisSetDidNotRegister(t *testing.T) {
	metrics := tools.NewMetrics()
	minter, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(minter,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})

	// Chats only: list_review_threads is a real tool name, but not on this set.
	ts := tools.NewToolSet(tools.Deps{
		Resolver: res,
		Chats:    &spyRenamer{},
		Metrics:  metrics,
	}, "RUN", minter.Mint("RUN"))

	_, err = ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.Error(t, err)

	snap := metrics.Snapshot()
	require.Equal(t, tools.ToolStat{Calls: 1, Failures: 1}, snap[tools.UnknownToolMetric])
	require.NotContains(t, snap, "list_review_threads")
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
