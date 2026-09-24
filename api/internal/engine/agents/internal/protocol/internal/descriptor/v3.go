package descriptor

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema"
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
		var glyph *spec.AlternationGlyphError
		if errors.As(err, &glyph) {
			// id: precedes events: in the document, but decode order is not a
			// guarantee worth leaning on — a bare best-effort re-read of just
			// the id field names the descriptor even if the strict decode
			// above never got that far.
			return nil, fmt.Errorf("descriptor %q: %w", probeID(raw), glyph)
		}
		return nil, fmt.Errorf("descriptor: parse: %w", err)
	}
	if d.ID == "" {
		// Wraps ErrInvalid: callers switch on that sentinel to tell "this provider is
		// unusable" from "something went wrong reading it", and a descriptor with no
		// id is the former.
		return nil, fmt.Errorf("%w: missing id", ErrInvalid)
	}

	vocab, err := schema.Load()
	if err != nil {
		return nil, err
	}

	// An inbound event's canonical fields are the KEYS of map:. An outbound event has
	// no map: — its payload is built by send: (or, for one that must first establish
	// a session, by fresh:/resume:/action:'s own nested send: trees), whose keys are
	// the PROVIDER's field names and whose values reference canonical names as
	// {braced} templates. So the direction is inverted and the canonical set has to
	// be read out of the values.
	maps := make(map[string]map[string][]string, len(d.Events))
	for name, e := range d.Events {
		switch {
		case !e.Out.Empty():
			maps[name] = wrapPresence(canonicalRefs(e.Send))
		case len(e.Fresh) > 0 || len(e.Resume) > 0 || len(e.Action) > 0:
			maps[name] = wrapPresence(canonicalRefsFromSteps(e.Fresh, e.Resume, e.Action))
		case e.HasChannelBlocks():
			// A channel-scoped event's canonical fields are split across its
			// api:/hooks: blocks; the vocabulary check needs the UNION so a bad
			// field name in EITHER channel's map: is caught, not just the one
			// that happens to run first.
			maps[name] = mergedChannelFields(e)
		default:
			maps[name] = e.Map
		}
	}
	if err := vocab.Validate(d.ID, maps); err != nil {
		return nil, fmt.Errorf("descriptor: %w", err)
	}

	for _, name := range sortedEventNames(d.Events) {
		if err := checkChannelSplit(d.ID, name, d.Events[name]); err != nil {
			return nil, err
		}
		if err := checkEvent(vocab, d.ID, name, d.Events[name]); err != nil {
			return nil, err
		}
	}
	return &d, nil
}

// mergedChannelFields unions a channel-scoped event's api: and hooks: field
// maps into one table, for the vocabulary's required/optional check — which
// key's PATHS win when both channels map the same canonical name does not
// matter here, only that the canonical name itself was declared somewhere.
func mergedChannelFields(e spec.EventSpec) map[string][]string {
	out := map[string][]string{}
	if e.API != nil {
		for k, v := range e.API.Map {
			out[k] = v
		}
	}
	if e.Hooks != nil {
		for k, v := range e.Hooks.Map {
			out[k] = v
		}
	}
	return out
}

// wrapPresence lifts a presence-only field table (an outbound event's
// canonicalRefs/canonicalRefsFromSteps, whose values are send templates, not
// mapping paths) into the shape the vocabulary check shares with an inbound
// event's real FieldMap. Only key presence and non-emptiness are ever read.
func wrapPresence(m map[string]string) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = []string{v}
	}
	return out
}

// probeID best-effort reads just the top-level id: field out of raw, for an error
// that must name the descriptor even though the strict typed decode already failed.
func probeID(raw []byte) string {
	var probe struct {
		ID string `yaml:"id"`
	}
	_ = yaml.Unmarshal(raw, &probe)
	return probe.ID
}

// checkChannelSplit refuses an event that mixes the legacy flat form
// (in:/out:/ask:/when:/map: at the event's own level) with the channel-split
// form (api:/hooks: blocks) — the two are mutually exclusive spellings of
// where an event's shape lives, and mixing them leaves no rule for which one
// resolution should read.
func checkChannelSplit(providerID, name string, e spec.EventSpec) error {
	if !e.HasChannelBlocks() {
		return nil
	}
	if !e.In.Empty() || !e.Out.Empty() || !e.Ask.Empty() {
		return fmt.Errorf(
			"descriptor: %s: event %q declares both a flat in:/out:/ask: and a "+
				"channel block (api:/hooks:) — pick one form", providerID, name,
		)
	}
	if len(e.When) > 0 || len(e.Map) > 0 {
		return fmt.Errorf(
			"descriptor: %s: event %q declares channel blocks but also a flat "+
				"when:/map: — pick one form", providerID, name,
		)
	}
	for _, channel := range []spec.Channel{spec.ChannelAPI, spec.ChannelHooks} {
		block := e.API
		if channel == spec.ChannelHooks {
			block = e.Hooks
		}
		if block != nil && !block.In.Empty() && !block.Ask.Empty() {
			return fmt.Errorf(
				"descriptor: %s: event %q's %s: block declares both in: and ask: — pick one",
				providerID, name, channel,
			)
		}
		if block != nil {
			if err := checkByWire(providerID, name, channel, block); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkByWire refuses a by_wire: entry keyed by a name the block itself does
// not answer to — a typo here would silently inject nothing and leave the
// field it was meant to derive unmapped, the same class of invisible gap
// required:/fixtures: exist to catch elsewhere.
func checkByWire(providerID, name string, channel spec.Channel, block *spec.ChannelBlock) error {
	if len(block.ByWire) == 0 {
		return nil
	}
	wire := block.Ask
	if wire.Empty() {
		wire = block.In
	}
	for key := range block.ByWire {
		if !wire.Has(key) {
			return fmt.Errorf(
				"descriptor: %s: event %q's %s: block's by_wire: names %q, "+
					"which is not one of its own in:/ask: wire names",
				providerID, name, channel, key,
			)
		}
	}
	return nil
}

// checkEvent enforces the rules the field table cannot express: that an event names a
// wire event, that it names it in the direction the vocabulary declares, and that its
// reply template only defines decisions the event accepts.
func checkEvent(vocab schema.Vocabulary, providerID, name string, e spec.EventSpec) error {
	rule := vocab.Events[name] // presence already proven by Validate

	// AnyWireEvent, not WireEvent: a channel-scoped event names nothing at
	// its OWN top level by construction — its wire name lives inside
	// api:/hooks: — so this must look there too, or every channel-scoped
	// event would fail load as "names nothing".
	wire, direction := e.AnyWireEvent()
	if wire.Empty() {
		if len(e.Fresh) == 0 && len(e.Resume) == 0 && len(e.Action) == 0 {
			return fmt.Errorf(
				"descriptor: %s: event %q declares no in:, out:, ask:, or fresh:/resume:/action: — it names nothing",
				providerID, name,
			)
		}
		// A Fresh/Resume/Action event names several wire calls, not one — the
		// event as a whole is still an OUTBOUND fact (Crowbar is the one
		// acting), same as an ordinary out: event, just without a single
		// name to check the direction table against.
		direction = "out"
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

// canonicalRefs pulls the {braced} canonical names an outbound event's send templates
// reference, so the same required/optional table validates both directions.
func canonicalRefs(send map[string]string) map[string]string {
	out := map[string]string{}
	for _, tmpl := range send {
		rest := tmpl
		for {
			open := strings.IndexByte(rest, '{')
			if open < 0 {
				break
			}
			rest = rest[open+1:]
			close := strings.IndexByte(rest, '}')
			if close < 0 {
				break
			}
			if name := rest[:close]; name != "" {
				out[name] = tmpl
			}
			rest = rest[close+1:]
		}
	}
	return out
}

// canonicalRefsFromSteps is canonicalRefs' counterpart for an event whose
// outbound payload is a Fresh/Resume/Action sequence rather than a single flat
// send: — each step's Send is an arbitrary (YAML-shaped) tree, so every string
// leaf at any depth is walked, not just a flat map's top-level values.
func canonicalRefsFromSteps(stepLists ...[]spec.CallStep) map[string]string {
	out := map[string]string{}
	for _, steps := range stepLists {
		for _, step := range steps {
			collectCanonicalRefs(step.Send, out)
		}
	}
	return out
}

func collectCanonicalRefs(node any, out map[string]string) {
	switch v := node.(type) {
	case string:
		rest := v
		for {
			open := strings.IndexByte(rest, '{')
			if open < 0 {
				return
			}
			rest = rest[open+1:]
			close := strings.IndexByte(rest, '}')
			if close < 0 {
				return
			}
			if name := rest[:close]; name != "" {
				out[name] = v
			}
			rest = rest[close+1:]
		}
	case map[string]any:
		for _, val := range v {
			collectCanonicalRefs(val, out)
		}
	case []any:
		for _, val := range v {
			collectCanonicalRefs(val, out)
		}
	}
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
