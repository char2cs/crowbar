package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type promptSubmit struct{}

func (promptSubmit) Name() string { return "prompt_submit" }

func (r promptSubmit) Check(d *spec.Descriptor) error {
	ps := d.Presentation.PromptSubmit
	if ps == nil {
		return nil
	}

	if ps.Strategy != spec.DeliveryRestartTUI {
		return invalid(d.ID,
			"presentation.prompt_submit has unsupported strategy %q", ps.Strategy)
	}

	if d.Session.Resume == nil {
		return invalid(d.ID, "presentation.prompt_submit requires session.resume")
	}
	if err := r.checkSteps(d.ID, "fresh", ps.Fresh); err != nil {
		return err
	}
	if err := r.checkSteps(d.ID, "resume", ps.Resume); err != nil {
		return err
	}
	return r.checkLeadingSigils(d.ID, ps.LeadingSigils)
}

func (promptSubmit) checkLeadingSigils(id string, sigils *spec.LeadingSigilsSpec) error {
	if sigils == nil {
		return nil
	}
	if len(sigils.Chars) == 0 {
		return invalid(id, "presentation.prompt_submit.leading_sigils.chars is empty")
	}
	for _, char := range sigils.Chars {
		if char == "" {
			return invalid(id, "presentation.prompt_submit.leading_sigils.chars holds an empty entry")
		}
	}
	if sigils.Escape == "" {
		return invalid(id, "presentation.prompt_submit.leading_sigils.escape is required")
	}
	// An escape that itself starts with a declared sigil would hand the CLI the
	// very gesture it is there to prevent.
	for _, char := range sigils.Chars {
		if strings.HasPrefix(sigils.Escape, char) {
			return invalid(id,
				"presentation.prompt_submit.leading_sigils.escape starts with the sigil %q it must hide", char)
		}
	}
	return nil
}

func (promptSubmit) checkSteps(id, name string, steps []spec.InjectStep) error {
	if len(steps) == 0 {
		return invalid(id, "presentation.prompt_submit.%s is empty", name)
	}
	messageCount := 0
	for _, step := range steps {
		if step.Verb != "pass_arg" {
			return invalid(id, "presentation.prompt_submit.%s may only pass argv", name)
		}
		for _, value := range step.Args {
			messageCount += strings.Count(spec.ArgString(value), "{message}")
		}
	}
	if messageCount != 1 {
		return invalid(id, "presentation.prompt_submit.%s must place {message} exactly once", name)
	}
	return nil
}
