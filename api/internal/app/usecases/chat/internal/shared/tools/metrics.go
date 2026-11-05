package tools

import "sync"

// ToolStat is the call/failure count recorded for one tool name.
type ToolStat struct {
	Calls    int
	Failures int
}

// Metrics counts calls and failures per tool across the daemon's lifetime; a failure (e.g. out of scope)
// is kept as the most telling datum. Unlike other Deps ports, nil never suppresses a tool: every method
// is nil-receiver safe, since losing observability is no reason to lose capability.
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

// Snapshot returns a copy of every tool's counters, so a caller cannot corrupt the live ones. A nil
// receiver returns nil, matching Record.
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
