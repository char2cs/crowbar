package tools

import "sync"

type ToolStat struct {
	Calls    int
	Failures int
}

type Metrics struct {
	mu    sync.Mutex
	stats map[string]ToolStat
}

func NewMetrics() *Metrics {
	return &Metrics{stats: map[string]ToolStat{}}
}

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
