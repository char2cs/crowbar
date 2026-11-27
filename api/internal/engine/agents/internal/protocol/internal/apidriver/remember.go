package apidriver

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type frameValue struct {
	text    string
	literal bool
}

func (drv *Driver) remember(method string, params map[string]any) {
	for _, entry := range drv.d.Runtime.API.Remember {
		if !entry.In.Has(method) || !mapping.Match(params, entry.When) {
			continue
		}
		for name, path := range entry.Capture {
			if v, literal, ok := mapping.Captured(params, mapping.Paths(path)); ok {
				drv.mu.Lock()
				drv.frameValues[name] = frameValue{text: v, literal: literal}
				drv.mu.Unlock()
			}
		}
	}
}

func (drv *Driver) capturedValues() map[string]any {
	drv.mu.Lock()
	defer drv.mu.Unlock()
	if len(drv.frameValues) == 0 {
		return nil
	}
	out := make(map[string]string, len(drv.frameValues))
	for name, v := range drv.frameValues {
		out[name] = v.text
	}
	return map[string]any{spec.CapturedNamespace: out}
}

func (drv *Driver) addFrameValues(values map[string]string, literals map[string]bool) {
	for name, v := range drv.frameValues {
		key := spec.CapturedNamespace + "." + name
		values[key] = v.text
		if v.literal {
			literals[key] = true
		}
	}
}
