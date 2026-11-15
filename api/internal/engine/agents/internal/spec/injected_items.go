package spec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PromptMatch opens an injected prompt by a regexp where no literal prefix
// can: the text after the prefix varies. It is compiled when the descriptor loads.
type PromptMatch struct {
	re *regexp.Regexp
}

func (m *PromptMatch) UnmarshalYAML(unmarshal func(any) error) error {
	var src string
	if err := unmarshal(&src); err != nil {
		return err
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return fmt.Errorf("match %q: %w", src, err)
	}
	m.re = re
	return nil
}

// Anchored reports whether m compiled and starts at the beginning of the text,
// so a person quoting the notice mid-sentence never matches.
func (m *PromptMatch) Anchored() bool {
	return m != nil && m.re != nil && strings.HasPrefix(m.re.String(), "^")
}

func (m *PromptMatch) Matches(text string) bool {
	return m != nil && m.re != nil && m.re.MatchString(text)
}

// TaskPattern finds every background task a prompt names: each match of its
// pattern is one, by the named groups id (required), exit_code and status.
type TaskPattern struct {
	re *regexp.Regexp
}

// TaskRef is one task a prompt names. Status is the provider's word for how it
// ended, "" when the match gave none; ExitCode is nil likewise.
type TaskRef struct {
	ID, Status string
	ExitCode   *int
}

func (p *TaskPattern) UnmarshalYAML(unmarshal func(any) error) error {
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
	if re.SubexpIndex("id") < 0 {
		return fmt.Errorf("pattern %q: no (?P<id>...) group", raw.Pattern)
	}
	p.re = re
	return nil
}

// Valid reports whether p compiled with its id group.
func (p TaskPattern) Valid() bool { return p.re != nil }

// FindAll is every task prompt names by p, in order.
func (p TaskPattern) FindAll(prompt string) []TaskRef {
	if p.re == nil {
		return nil
	}
	var refs []TaskRef
	for _, m := range p.re.FindAllStringSubmatch(prompt, -1) {
		ref := TaskRef{ID: m[p.re.SubexpIndex("id")]}
		if i := p.re.SubexpIndex("status"); i >= 0 {
			ref.Status = m[i]
		}
		if i := p.re.SubexpIndex("exit_code"); i >= 0 {
			if code, err := strconv.Atoi(m[i]); err == nil {
				ref.ExitCode = &code
			}
		}
		refs = append(refs, ref)
	}
	return refs
}
