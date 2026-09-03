// Package schema loads Crowbar's canonical conversation vocabulary and validates a
// provider descriptor's event table against it.
//
// The vocabulary is data (vocabulary.yaml, embedded here because go:embed cannot
// reach a parent directory — this package owns it and descriptor/ reads it through
// Load) rather than Go constants so that
// adding a PROVIDER needs no Go. It is closed in the other direction: adding an EVENT
// is a Crowbar capability change and does need Go on the consuming side.
package schema

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed vocabulary.yaml
var vocabularyYAML []byte

// familySuffix marks a prefix FAMILY in an optional list: `suggestion_label.*` admits
// any field under `suggestion_label.`, because those keys are provider vocabulary.
const familySuffix = ".*"

type EventRule struct {
	Direction string   `yaml:"direction"`
	Required  []string `yaml:"required"`
	Optional  []string `yaml:"optional"`
	// Extras are sibling keys on the event that carry structure rather than a field
	// map (telemetry.rate_limits, permission.answers_into). Their shape is the
	// event's own business, not this table's.
	Extras []string `yaml:"extras"`
	// Replies names the decisions an `ask` event's reply template may define.
	Replies []string `yaml:"replies"`
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

// Validate checks one provider's event table.
//
// A provider may map a SUBSET of the vocabulary — capability is key-presence — but
// every event it does map must be a known one, must carry that event's required
// fields, and must not carry a field the event does not declare. The last rule is what
// turns a typo from "maps silently to nothing" into a startup failure.
func (v Vocabulary) Validate(providerID string, events map[string]map[string]string) error {
	for _, name := range sortedKeys(events) {
		rule, ok := v.Events[name]
		if !ok {
			return fmt.Errorf("%s: unknown event %q: the vocabulary is closed", providerID, name)
		}
		fields := events[name]
		for _, req := range rule.Required {
			if fields[req] == "" {
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

// admits reports whether field is one this event declares, either exactly or as a
// member of a prefix family.
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

// sortedKeys makes every error deterministic. Without it the message a bad descriptor
// produces changes between runs and the test asserting on it is flaky.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
