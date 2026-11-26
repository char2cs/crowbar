package apidriver

import (
	"maps"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/registry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// injectAt is the descriptor's inject step for lifecycle moment at, nil without one.
func (drv *Driver) injectAt(at string) *spec.InjectSpec {
	for i := range drv.d.Inject {
		if drv.d.Inject[i].At == at {
			return &drv.d.Inject[i]
		}
	}
	return nil
}

// armLead queues the context document the descriptor leads the session's first prompt with.
// A step that declares no prompt_lead, or a document that renders empty, queues nothing.
func (drv *Driver) armLead(values map[string]string) {
	step := drv.injectAt("context")
	if step == nil || step.PromptLead == "" {
		return
	}
	// Untrimmed: the echo of the prompt is matched against the document as Crowbar registered it.
	lead := outbound.Substitute(step.PromptLead, values)
	if strings.TrimSpace(lead) == "" {
		return
	}
	drv.mu.Lock()
	drv.lead = lead
	drv.mu.Unlock()
}

// leadValues is values with the queued lead in front of its text, and the lead that was
// put there. The lead is spent only once the prompt it rode in on is accepted.
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

// spendLead retires lead unless a newer one was queued meanwhile.
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
