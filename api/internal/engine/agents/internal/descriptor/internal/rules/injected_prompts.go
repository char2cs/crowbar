package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type injectedPrompts struct{}

func (injectedPrompts) Name() string { return "injected_prompts" }

func (injectedPrompts) Check(d *spec.Descriptor) error {
	for i, p := range d.InjectedPrompts {
		// A needle is compared as a prefix of the raw prompt after leading
		// whitespace is trimmed (see the promptorigin package). An empty needle is
		// a prefix of every string, and a whitespace-only one is a prefix of every
		// string the matcher has already trimmed — so either would file EVERY
		// prompt the user ever sends as the harness's. That is the one failure this
		// feature must never be able to produce, and a descriptor is the one place
		// it could be introduced without a daemon build, so it is refused here
		// rather than skipped at match time.
		if strings.TrimSpace(p.Needle) == "" {
			return invalid(d.ID, "injected_prompts[%d]: needle must not be empty or whitespace", i)
		}
		if p.Needle != strings.TrimLeft(p.Needle, " \t\r\n") {
			// Leading whitespace on the needle can never match, because the matcher
			// trims it off the prompt first. Rejected rather than trimmed, so a
			// declaration that cannot fire is not shipped looking like one that can.
			return invalid(d.ID, "injected_prompts[%d]: needle must not start with whitespace", i)
		}
		if p.Kind == "" {
			continue
		}
		if _, ok := spec.InjectedPromptKinds[p.Kind]; !ok {
			// Rejected rather than downgraded to the generic case, exactly as an
			// unknown terminal-prompt kind is. A typo that silently became "injected,
			// unidentified" would look like a working descriptor forever.
			return invalid(d.ID, "injected_prompts[%d]: unknown kind %q", i, p.Kind)
		}
	}
	return nil
}
