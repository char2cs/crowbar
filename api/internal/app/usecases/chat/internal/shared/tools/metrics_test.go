package tools_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
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
