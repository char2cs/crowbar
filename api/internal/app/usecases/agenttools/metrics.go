package agenttools

import "sync"

// ToolStat is the call/failure count recorded for one tool name.
type ToolStat struct {
	Calls    int
	Failures int
}

// Metrics counts how many times each tool was called and how many of those
// calls failed, across every ToolSet built for every runner in the daemon's
// lifetime.
//
// It is the answer to "do agents actually use these tools?" — the shell
// command this surface replaces is known to be ignored by real models, and
// without a count that stays a feeling instead of a number. A failed call is
// recorded too, and deliberately not dropped: an unauthorized or out-of-scope
// attempt is the most interesting datum this can produce, since it means an
// agent tried something it should not have, or that scope resolution itself is
// misconfigured.
//
// Unlike every other Deps port, a nil *Metrics must never suppress a tool's
// registration — losing observability is not a reason to lose capability. Every
// method is therefore safe to call on a nil receiver, which lets ToolSet.Call
// invoke Record unconditionally instead of nil-checking at every call site.
type Metrics struct {
	mu    sync.Mutex
	stats map[string]ToolStat
}

// NewMetrics returns an empty Metrics ready to record calls.
func NewMetrics() *Metrics {
	return &Metrics{stats: map[string]ToolStat{}}
}

// Record counts one call to tool, and one failure of it when ok is false. A nil
// receiver is a no-op so a daemon wired with Deps.Metrics == nil still runs
// every tool at full capability, just uncounted.
func (m *Metrics) Record(tool string, ok bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	stat := m.stats[tool]
	stat.Calls++
	if !ok {
		stat.Failures++
	}
	m.stats[tool] = stat
}

// Snapshot returns a point-in-time copy of every tool's counters. It is a copy
// specifically so a caller mutating the returned map — a test asserting on it,
// a future HTTP handler serializing it — can never corrupt the live counters. A
// nil receiver returns nil rather than panicking, matching Record's fail-open
// behavior.
func (m *Metrics) Snapshot() map[string]ToolStat {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]ToolStat, len(m.stats))
	for tool, stat := range m.stats {
		out[tool] = stat
	}
	return out
}
