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
	switch ps.Strategy {
	case spec.DeliveryRestartTUI:
	case spec.DeliveryRewakeHook:
		if err := r.checkRewake(d, ps.Rewake); err != nil {
			return err
		}
	default:
		return invalid(d.ID,
			"presentation.prompt_submit has unsupported strategy %q", ps.Strategy)
	}
	// Both strategies fall back to a restart for the first message of a session,
	// so the argv steps are required either way.
	if d.Session.Resume == nil {
		return invalid(d.ID, "presentation.prompt_submit requires session.resume")
	}
	if err := r.checkSteps(d.ID, "fresh", ps.Fresh); err != nil {
		return err
	}
	return r.checkSteps(d.ID, "resume", ps.Resume)
}

// checkRewake refuses a rewake declaration that could not tell a prompt Crowbar
// delivered from one the provider's own harness wrote.
//
// Every check here guards the SAME failure from a different side: a wrapper the
// engine cannot unwrap, or can unwrap without proving who sent it, files the
// user's own messages under the harness's name and deletes them from their side
// of the conversation. A descriptor is the one place that could be introduced
// without a daemon build, so it is refused at load rather than at match time.
func (promptSubmit) checkRewake(d *spec.Descriptor, rw *spec.RewakeSpec) error {
	if rw == nil || strings.TrimSpace(rw.Sentinel) == "" {
		return invalid(d.ID,
			"presentation.prompt_submit strategy %q requires rewake.sentinel",
			spec.DeliveryRewakeHook)
	}
	if strings.TrimSpace(rw.Summary) == "" {
		return invalid(d.ID,
			"presentation.prompt_submit strategy %q requires rewake.summary",
			spec.DeliveryRewakeHook)
	}
	if rw.Strip == "" {
		return invalid(d.ID,
			"presentation.prompt_submit strategy %q requires rewake.strip",
			spec.DeliveryRewakeHook)
	}
	// The sentinel must be INSIDE the pattern. A strip that matched without it
	// would unwrap anything shaped like the wrapper, including a harness
	// notification, and hand its body back as something the user typed.
	if !strings.Contains(rw.Strip, "{sentinel}") {
		return invalid(d.ID,
			"presentation.prompt_submit.rewake.strip must interpolate {sentinel}")
	}
	if err := requireNamedGroup(rw.StripPattern(), "message"); err != nil {
		return invalid(d.ID, "presentation.prompt_submit.rewake.strip: %v", err)
	}
	// A wake status of 0 is the shell's word for SUCCESS, which is exactly the
	// status a provider ignores. Declaring it would register a collector that can
	// never deliver anything and a chat that silently never leaves the floor.
	if rw.WakeStatus <= 0 || rw.WakeStatus > 255 {
		return invalid(d.ID,
			"presentation.prompt_submit.rewake.wake_status must be a process exit status between 1 and 255, got %d",
			rw.WakeStatus)
	}
	return nil
}

// checkSteps enforces that a prompt reaches the CLI as argv and nothing else, and
// that the message appears exactly once. Twice would deliver the prompt twice;
// never would spawn a CLI that silently drops what the user typed.
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
