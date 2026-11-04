package rules

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type terminalPrompts struct{}

func (terminalPrompts) Name() string { return "terminal_prompts" }

func (terminalPrompts) Check(d *spec.Descriptor) error {
	for i, p := range d.TerminalPrompts {
		if !hasContent(p.Needle) {
			return invalid(d.ID, "terminal_prompts[%d]: needle must contain letters or digits", i)
		}
		if p.Kind != "" {
			if _, ok := spec.TerminalPromptKinds[p.Kind]; !ok {
				return invalid(d.ID, "terminal_prompts[%d]: unknown kind %q", i, p.Kind)
			}
		}
		if err := checkAnswers(d.ID, i, p.Answers); err != nil {
			return err
		}
	}
	return nil
}

func checkAnswers(providerID string, prompt int, answers []spec.TerminalPromptAnswerSpec) error {
	seen := make(map[string]struct{}, len(answers))
	for j, a := range answers {
		if a.ID == "" || a.Label == "" || a.Keys == "" {
			return invalid(providerID, "terminal_prompts[%d].answers[%d]: id, label and keys are required", prompt, j)
		}
		if _, dup := seen[a.ID]; dup {
			return invalid(providerID, "terminal_prompts[%d].answers[%d]: duplicate id %q", prompt, j, a.ID)
		}
		seen[a.ID] = struct{}{}
	}
	return nil
}

func hasContent(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}
