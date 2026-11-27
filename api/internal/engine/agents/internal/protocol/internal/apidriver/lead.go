package apidriver

import (
	"maps"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func (drv *Driver) injectAt(at string) *spec.InjectSpec {
	for i := range drv.d.Inject {
		if drv.d.Inject[i].At == at {
			return &drv.d.Inject[i]
		}
	}
	return nil
}

func (drv *Driver) armLead(values map[string]string) {
	step := drv.injectAt("context")
	if step == nil || step.PromptLead == "" {
		return
	}
	lead := outbound.Substitute(step.PromptLead, values)
	if strings.TrimSpace(lead) == "" {
		return
	}
	drv.mu.Lock()
	drv.lead = lead
	drv.mu.Unlock()
}

func (drv *Driver) leadValues(values map[string]string) (map[string]string, string) {
	drv.mu.Lock()
	lead := drv.lead
	drv.mu.Unlock()
	if lead == "" || values["text"] == "" {
		return values, ""
	}
	led := maps.Clone(values)
	led["text"] = lead + registry.MergedSeparator + values["text"]
	return led, lead
}

func (drv *Driver) spendLead(lead string) {
	if lead == "" {
		return
	}
	drv.mu.Lock()
	defer drv.mu.Unlock()
	if drv.lead == lead {
		drv.lead = ""
	}
}
