package rules

import (
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

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
	if err := r.checkLeadingSigils(d.ID, ps.LeadingSigils); err != nil {
		return err
	}
	if err := checkSteer(d, ps.Steer); err != nil {
		return err
	}
	return checkType(d.ID, ps.Type)
}

func checkType(id string, typing *spec.PromptTypeSpec) error {
	if typing == nil {
		return nil
	}
	if typing.Input != spec.InputKeys {
		return invalid(id, "presentation.prompt_submit.type.input %q is not a known input mode", typing.Input)
	}
	if typing.ChunkBytes < utf8.UTFMax {
		return invalid(id, "presentation.prompt_submit.type.chunk_bytes must be at least %d", utf8.UTFMax)
	}
	if typing.ChunkIntervalMS < 0 {
		return invalid(id, "presentation.prompt_submit.type.chunk_interval_ms is negative")
	}
	if typing.Submit == "" {
		return invalid(id, "presentation.prompt_submit.type.submit is required")
	}
	if typing.Composer.RowPrefix == "" {
		return invalid(id, "presentation.prompt_submit.type.composer.row_prefix is required")
	}
	if g := typing.Composer.Ghost; g != "" && g != spec.GhostFaint {
		return invalid(id, "presentation.prompt_submit.type.composer.ghost %q is not a known ghost style", g)
	}
	if slices.Contains(typing.SkipPrefixes, "") {
		return invalid(id, "presentation.prompt_submit.type.skip_prefixes holds an empty entry")
	}
	if slices.Contains(typing.SkipSuffixes, "") {
		return invalid(id, "presentation.prompt_submit.type.skip_suffixes holds an empty entry")
	}
	return nil
}

func checkSteer(d *spec.Descriptor, steer *spec.SteerSpec) error {
	if steer == nil {
		return nil
	}
	if _, declared := d.Events[spec.HookTurnStop]; !declared {
		return invalid(d.ID, "presentation.prompt_submit.steer needs the %s event", spec.HookTurnStop)
	}
	if strings.Count(steer.Frame, "{message}") != 1 {
		return invalid(d.ID, "presentation.prompt_submit.steer.frame must place {message} exactly once")
	}
	if strings.Count(steer.Reply, "{message_json}") != 1 {
		return invalid(d.ID, "presentation.prompt_submit.steer.reply must place {message_json} exactly once")
	}
	var probe any
	if err := json.Unmarshal([]byte(strings.Replace(steer.Reply, "{message_json}", `"x"`, 1)), &probe); err != nil {
		return invalid(d.ID, "presentation.prompt_submit.steer.reply is not JSON once {message_json} is filled: %v", err)
	}
	for _, prefix := range steer.SkipPrefixes {
		if prefix == "" {
			return invalid(d.ID, "presentation.prompt_submit.steer.skip_prefixes holds an empty entry")
		}
	}
	return nil
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
