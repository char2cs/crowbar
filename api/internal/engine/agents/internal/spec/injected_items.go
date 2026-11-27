package spec

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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

func (m *PromptMatch) Anchored() bool {
	return m != nil && m.re != nil && strings.HasPrefix(m.re.String(), "^")
}

func (m *PromptMatch) Matches(text string) bool {
	return m != nil && m.re != nil && m.re.MatchString(text)
}

type TaskPattern struct {
	re *regexp.Regexp
}

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

func (p TaskPattern) Valid() bool { return p.re != nil }

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
