package rules

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type contextChannel struct{}

func (contextChannel) Name() string { return "context_channel" }

// Check refuses an api-transport descriptor that opens sessions yet never says how Crowbar's
// context (its tools preamble, and a replaced session's record) reaches them: such a session
// would start without it, and the chat would lose its conversation.
func (contextChannel) Check(d *spec.Descriptor) error {
	for i, step := range d.Inject {
		if err := checkInjectStep(d.ID, i, step); err != nil {
			return err
		}
	}
	if d.Runtime.Transport != "api" || len(d.Events["prompt"].Fresh) == 0 || carriesContext(d) {
		return nil
	}
	return invalid(d.ID, "no step hands a new session {context}: set it in prompt.fresh, or declare "+
		"inject: [{at: context, prompt_lead: \"{context}\"}] to lead the session's first prompt with it")
}

func carriesContext(d *spec.Descriptor) bool {
	if len(d.ContextInject) > 0 {
		return true
	}
	if slices.ContainsFunc(d.Events["prompt"].Fresh, func(step spec.CallStep) bool { return mentions(step.Send, "{context}") }) {
		return true
	}
	return slices.ContainsFunc(d.Inject, func(step spec.InjectSpec) bool {
		return step.At == "context" && (step.Call != "" || step.PromptLead != "")
	})
}

// checkInjectStep keeps a prompt_lead to what it can mean: the context moment, alone,
// naming the document it leads with.
func checkInjectStep(id string, i int, step spec.InjectSpec) error {
	if step.PromptLead == "" {
		return nil
	}
	if step.At != "context" {
		return invalid(id, "inject[%d].prompt_lead is only for at: context, not %q", i, step.At)
	}
	if step.Call != "" || len(step.Send) > 0 || len(step.Steps) > 0 {
		return invalid(id, "inject[%d] sets prompt_lead and a call: pick one", i)
	}
	if !mentions(step.PromptLead, "{context}") {
		return invalid(id, "inject[%d].prompt_lead never names {context}", i)
	}
	return nil
}
