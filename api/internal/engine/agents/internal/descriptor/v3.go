package descriptor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/schema"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// ParseV3 unmarshals a v3 descriptor and validates its event table against Crowbar's
// canonical vocabulary.
//
// Validation happens at LOAD so a bad descriptor fails when the daemon starts, never
// mid-conversation. Everything checked here compiles fine and would otherwise surface
// as a field that silently maps to nothing.
func ParseV3(raw []byte) (*spec.Descriptor, error) {
	var d spec.Descriptor
	if err := yaml.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("descriptor: parse: %w", err)
	}
	if d.ID == "" {
		return nil, fmt.Errorf("descriptor: missing id")
	}

	vocab, err := schema.Load()
	if err != nil {
		return nil, err
	}

	maps := make(map[string]map[string]string, len(d.Events))
	for name, e := range d.Events {
		maps[name] = e.Map
	}
	if err := vocab.Validate(d.ID, maps); err != nil {
		return nil, fmt.Errorf("descriptor: %w", err)
	}

	for _, name := range sortedEventNames(d.Events) {
		if err := checkEvent(vocab, d.ID, name, d.Events[name]); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// checkEvent enforces the rules the field table cannot express: that an event names a
// wire event, that it names it in the direction the vocabulary declares, and that its
// reply template only defines decisions the event accepts.
func checkEvent(vocab schema.Vocabulary, providerID, name string, e spec.EventSpec) error {
	rule := vocab.Events[name] // presence already proven by Validate

	wire, direction := e.WireEvent()
	if wire == "" {
		return fmt.Errorf(
			"descriptor: %s: event %q declares no in:, out: or ask: — it names nothing",
			providerID, name,
		)
	}
	if direction != rule.Direction {
		return fmt.Errorf(
			"descriptor: %s: event %q is declared with %s: but the vocabulary says it is %s",
			providerID, name, direction, rule.Direction,
		)
	}

	allowed := make(map[string]bool, len(rule.Replies))
	for _, r := range rule.Replies {
		allowed[r] = true
	}
	for _, got := range sortedKeys(e.Reply) {
		if !allowed[got] {
			return fmt.Errorf(
				"descriptor: %s: event %q declares no reply %q (accepts: %s)",
				providerID, name, got, strings.Join(rule.Replies, ", "),
			)
		}
	}
	return nil
}

// CheckProtocolVersion refuses a provider CLI whose protocol is outside the range the
// descriptor was written against.
//
// A descriptor with no declared range accepts anything, and a provider that reports no
// version is not gated — neither absence is evidence of a mismatch.
func CheckProtocolVersion(d *spec.Descriptor, actual string) error {
	if d.ProtocolVersion == nil || actual == "" {
		return nil
	}
	r := d.ProtocolVersion
	if (r.Min != "" && versionLess(actual, r.Min)) || (r.Max != "" && versionLess(r.Max, actual)) {
		return fmt.Errorf(
			"descriptor %s: provider protocol %s is outside the supported range [%s, %s]",
			d.ID, actual, r.Min, r.Max,
		)
	}
	return nil
}

// versionLess compares dotted-numeric versions component by component. A string
// compare gets "1.10" < "1.9" wrong, which is the whole reason this exists.
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := component(as, i), component(bs, i)
		if av != bv {
			return av < bv
		}
	}
	return false
}

func component(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[i]))
	if err != nil {
		return 0
	}
	return n
}

// Sorted iteration keeps every error deterministic; without it the message a bad
// descriptor produces changes between runs.
func sortedEventNames(m map[string]spec.EventSpec) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
