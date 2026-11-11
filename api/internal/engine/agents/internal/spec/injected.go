package spec

import (
	"fmt"
	"regexp"
)

const (
	InjectedPromptTaskNotification = "task_notification"
	// InjectedPromptSubagentResult is a subagent's result handed back to its
	// parent: the event is subagent_result, never a prompt.
	InjectedPromptSubagentResult = "subagent_result"
)

var InjectedPromptKinds = map[string]struct{}{
	InjectedPromptTaskNotification: {},
	InjectedPromptSubagentResult:   {},
}

type InjectedPromptSpec struct {
	Kind string `yaml:"kind"`

	Needle string `yaml:"needle"`

	// ScopeID, Status and Result read a subagent_result's fields out of the prompt.
	ScopeID *PromptPattern `yaml:"scope_id"`
	Status  *PromptPattern `yaml:"status"`
	Result  *PromptPattern `yaml:"result"`
}

// PromptPattern keeps the (?P<v>...) group of its pattern matched in a prompt;
// it is compiled when the descriptor loads.
type PromptPattern struct {
	re *regexp.Regexp
}

func (p *PromptPattern) UnmarshalYAML(unmarshal func(any) error) error {
	var raw struct {
		Pattern string `yaml:"pattern"`
	}
	if err := unmarshal(&raw); err != nil {
		return err
	}
	re, err := regexp.Compile(raw.Pattern)
	if err != nil {
		return fmt.Errorf("pattern %q: %w", raw.Pattern, err)
	}
	if re.SubexpIndex("v") < 0 {
		return fmt.Errorf("pattern %q: no (?P<v>...) group", raw.Pattern)
	}
	p.re = re
	return nil
}

// Find is the group in prompt; "" when p is nil or does not match.
func (p *PromptPattern) Find(prompt string) string {
	if p == nil || p.re == nil {
		return ""
	}
	m := p.re.FindStringSubmatch(prompt)
	if m == nil {
		return ""
	}
	return m[p.re.SubexpIndex("v")]
}
