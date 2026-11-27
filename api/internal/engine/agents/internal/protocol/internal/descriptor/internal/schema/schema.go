package schema

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed vocabulary.yaml
var vocabularyYAML []byte

const familySuffix = ".*"

type EventRule struct {
	Direction string   `yaml:"direction"`
	Required  []string `yaml:"required"`
	Optional  []string `yaml:"optional"`
	Extras    []string `yaml:"extras"`
	Replies   []string `yaml:"replies"`
}

type Vocabulary struct {
	Version int                  `yaml:"version"`
	Events  map[string]EventRule `yaml:"events"`
}

func Load() (Vocabulary, error) {
	v, err := parse(vocabularyYAML)
	if err != nil {
		return Vocabulary{}, err
	}
	if len(v.Events) == 0 {
		return Vocabulary{}, fmt.Errorf("schema: vocabulary declares no events")
	}
	for name, rule := range v.Events {
		switch rule.Direction {
		case "in", "out", "ask":
		default:
			return Vocabulary{}, fmt.Errorf(
				"schema: event %q has direction %q, want in|out|ask", name, rule.Direction,
			)
		}
	}
	return v, nil
}

func (v Vocabulary) Validate(providerID string, events map[string]map[string][]string) error {
	for _, name := range sortedKeys(events) {
		rule, ok := v.Events[name]
		if !ok {
			return fmt.Errorf("%s: unknown event %q: the vocabulary is closed", providerID, name)
		}
		fields := events[name]
		for _, req := range rule.Required {
			if len(fields[req]) == 0 {
				return fmt.Errorf("%s: event %q must map %q", providerID, name, req)
			}
		}
		for _, got := range sortedKeys(fields) {
			if !rule.admits(got) {
				return fmt.Errorf("%s: event %q declares no field %q", providerID, name, got)
			}
		}
	}
	return nil
}

func (r EventRule) admits(field string) bool {
	for _, allowed := range r.Required {
		if allowed == field {
			return true
		}
	}
	for _, allowed := range r.Optional {
		if family, ok := strings.CutSuffix(allowed, familySuffix); ok {
			if strings.HasPrefix(field, family+".") {
				return true
			}
			continue
		}
		if allowed == field {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
