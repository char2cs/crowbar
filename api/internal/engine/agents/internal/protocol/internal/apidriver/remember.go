package apidriver

import (
	"maps"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
)

// capturedKey is where a frame's own fields are joined by what earlier frames left,
// so an event's map reads it like any other path.
const capturedKey = "captured"

// remember keeps what the descriptor's remember: entries pick out of this frame; a
// null or absent path leaves the last value standing.
func (drv *Driver) remember(method string, params map[string]any) {
	for _, entry := range drv.d.Runtime.API.Remember {
		if !entry.In.Has(method) || !mapping.Match(params, entry.When) {
			continue
		}
		for name, path := range entry.Capture {
			if v, _, ok := mapping.Captured(params, mapping.Paths(path)); ok {
				drv.mu.Lock()
				drv.frameValues[name] = v
				drv.mu.Unlock()
			}
		}
	}
}

// capturedValues is a copy of what frames have left so far, nil while none has.
func (drv *Driver) capturedValues() map[string]any {
	drv.mu.Lock()
	defer drv.mu.Unlock()
	if len(drv.frameValues) == 0 {
		return nil
	}
	out := make(map[string]string, len(drv.frameValues))
	maps.Copy(out, drv.frameValues)
	return map[string]any{capturedKey: out}
}
