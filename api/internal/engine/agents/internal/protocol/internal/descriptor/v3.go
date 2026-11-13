package descriptor

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// ParseV0 is the schema 0 reader: it unmarshals a descriptor and validates its
// event table against Crowbar's canonical vocabulary at LOAD, so a bad descriptor
// fails at startup instead of silently mapping a field to nothing mid-conversation.
func ParseV0(raw []byte) (*spec.Descriptor, error) {
	d, err := decodeDescriptor(raw)
	if err != nil {
		return nil, err
	}

	vocab, err := schema.Load()
	if err != nil {
		return nil, err
	}

	maps := eventFieldPresence(d.Events)
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
	return d, nil
}

// decodeDescriptor unmarshals raw and rejects a descriptor with no id.
func decodeDescriptor(raw []byte) (*spec.Descriptor, error) {
	var d spec.Descriptor
	if err := yaml.Unmarshal(raw, &d); err != nil {
		var glyph *spec.AlternationGlyphError
		if errors.As(err, &glyph) {
			// Decode order is no guarantee that id: was read before the glyph
			// failed it, so a bare re-read of the id names the descriptor.
			id, idErr := probeID(raw)
			return nil, errors.Join(fmt.Errorf("descriptor %q: %w", id, glyph), idErr)
		}
		return nil, fmt.Errorf("descriptor: parse: %w", err)
	}
	if d.ID == "" {
		// Wraps ErrInvalid: callers switch on that sentinel to tell "this provider is
		// unusable" from "something went wrong reading it", and a descriptor with no
		// id is the former.
		return nil, fmt.Errorf("%w: missing id", ErrInvalid)
	}
	return &d, nil
}

// eventFieldPresence reads, per event, which canonical fields its descriptor declares.
func eventFieldPresence(events map[string]spec.EventSpec) map[string]map[string][]string {
	// An inbound event's canonical fields are the KEYS of map:. An outbound event's
	// send: trees are keyed by PROVIDER field names and reference canonical names as
	// {braced} templates, so its canonical set is read out of the values. A name some
	// step captures is the descriptor's own variable, not a vocabulary slot.
	captured := capturedNames(events)
	out := make(map[string]map[string][]string, len(events))
	for name, e := range events {
		switch {
		case !e.Out.Empty():
			refs := map[string]string{}
			collectCanonicalRefs(e.Send, refs)
			for ref := range refs {
				if captured[ref] {
					delete(refs, ref)
				}
			}
			out[name] = wrapPresence(refs)
		case len(e.Fresh) > 0 || len(e.Resume) > 0 || len(e.Action) > 0:
			out[name] = wrapPresence(canonicalRefsFromSteps(e.Fresh, e.Resume, e.Action))
		case e.HasChannelBlocks():
			// The UNION of the api:/hooks: blocks, so a bad field name in
			// either channel's map: is caught.
			out[name] = mergedChannelFields(e)
		default:
			out[name] = presence(e.Map)
		}
	}
	return out
}

// capturedNames is every value a Fresh/Resume/Action step captures from a response.
func capturedNames(events map[string]spec.EventSpec) map[string]bool {
	out := map[string]bool{}
	for _, e := range events {
		for _, steps := range [][]spec.CallStep{e.Fresh, e.Resume, e.Action} {
			for _, step := range steps {
				for name := range step.Capture {
					out[name] = true
				}
			}
		}
	}
	return out
}

// mergedChannelFields unions a channel-scoped event's api: and hooks: field maps
// for the vocabulary check, which reads only which canonical names are declared.
func mergedChannelFields(e spec.EventSpec) map[string][]string {
	out := map[string][]string{}
	if e.API != nil {
		maps.Copy(out, presence(e.API.Map))
	}
	if e.Hooks != nil {
		maps.Copy(out, presence(e.Hooks.Map))
	}
	return out
}

// presence reduces a field table to what the vocabulary check reads: which canonical
// fields are mapped, and to how many alternatives.
func presence(fields spec.FieldMap) map[string][]string {
	out := make(map[string][]string, len(fields))
	for k, v := range fields {
		out[k] = make([]string, len(v))
	}
	return out
}

// wrapPresence lifts an outbound event's template refs into the shape the
// vocabulary check shares with inbound FieldMaps; only key presence is read.
func wrapPresence(m map[string]string) map[string][]string {
	out := make(map[string][]string, len(m))
	for k, v := range m {
		out[k] = []string{v}
	}
	return out
}

// probeID reads just the top-level id: field out of raw, for an error that must
// name the descriptor even though the strict typed decode already failed.
func probeID(raw []byte) (string, error) {
	var probe struct {
		ID string `yaml:"id"`
	}
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return "", fmt.Errorf("descriptor: read id: %w", err)
	}
	return probe.ID, nil
}

// checkChannelSplit refuses an event mixing the legacy flat form (top-level
// in:/out:/ask:/when:/map:) with api:/hooks: blocks: no rule says which one wins.
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

// checkByWire refuses a by_wire: key the block does not answer to; a typo would
// otherwise silently inject nothing and leave the field unmapped.
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

	// AnyWireEvent, not WireEvent: a channel-scoped event's wire name lives
	// inside api:/hooks:, not at its own top level.
	wire, direction := e.AnyWireEvent()
	if wire.Empty() {
		if len(e.Fresh) == 0 && len(e.Resume) == 0 && len(e.Action) == 0 {
			return fmt.Errorf(
				"descriptor: %s: event %q declares no in:, out:, ask:, or fresh:/resume:/action: — it names nothing",
				providerID, name,
			)
		}
		// A Fresh/Resume/Action event names several calls but is still OUTBOUND
		// (Crowbar acts), just without a single name to check.
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

// CheckProtocolVersion refuses a provider CLI whose protocol is outside the descriptor's
// range. No declared range, or no reported version, is not evidence of a mismatch.
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

// canonicalRefsFromSteps pulls the {braced} canonical names a Fresh/Resume/Action
// step list references; Send is an arbitrary tree, so every string leaf is walked.
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
