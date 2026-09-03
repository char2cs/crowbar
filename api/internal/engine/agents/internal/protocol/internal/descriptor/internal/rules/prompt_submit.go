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
	return r.checkSteps(d.ID, "resume", ps.Resume)
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
