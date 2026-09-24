package descriptor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/schema"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var placeholderRe = regexp.MustCompile(`\{[a-z][a-z0-9_]*\}`)

const fixtureRoot = "../../testdata/fixtures"

// Every inbound field a v3 descriptor maps must resolve against RECORDED provider
// traffic. This is the only mechanism that catches a provider changing payload shape,
// and it is what closes the design spec's open question about unverified leaf paths.
//
// It has already earned its place: four paths written from the published JSON schema
// were wrong against real traffic — turn.lastAgentMessage and item.output do not
// exist, the delta carries no sequence, and tokenUsage nests under total/.
//
// A CHANNEL-SCOPED event (api:/hooks: blocks) has no field map or wire name at its
// OWN top level by construction — ev.WireEvent()/ev.Map are empty for it, always.
// Checking only those, the way this test did before the descriptor channel-split
// (docs/plans/2026-09-22-descriptor-channel-split.md), silently verified NOTHING
// for such an event: loadFixtures got zero wire names, logged "no fixture", and
// moved on — a false green, not a skip. Every channel a HasChannelBlocks() event
// declares is walked and checked on its own, through the SAME channel-aware
// accessors production resolution uses (WireEventFor/WhenFor/EventFieldsFor).
func TestV3Descriptors_ResolveAgainstRecordedTraffic(t *testing.T) {
	// Walks experimental/ too: a descriptor that is not shipped yet is exactly the one
	// whose paths are least proven, so it needs the replay most.
	files, err := v3Files()
	if err != nil {
		t.Skipf("no v3 descriptors yet: %v", err)
	}

	var checkedEvents int
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := descriptor.ParseV3(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		t.Run(d.ID, func(t *testing.T) {
			for name, ev := range d.Events {
				if ev.HasChannelBlocks() {
					checkedEvents += checkChannelBlockEvent(t, d, name, ev)
					continue
				}
				wire, direction := ev.WireEvent()
				if direction == "out" || wire.Empty() {
					// An outbound call has no inbound payload to replay — including a
					// Fresh/Resume/Action event (codex's own `prompt`), whose
					// WireEvent() is empty because it names no single in:/out:/ask:
					// at all (checkEvent's own "names nothing" rule already treats
					// that shape as direction "out").
					continue
				}
				// A legacy event answers the same on every channel; its recorded
				// fixtures follow the descriptor's own declared transport.
				channel := spec.ChannelAPI
				if d.TransportFor(name) == "hooks" {
					channel = spec.ChannelHooks
				}
				checkedEvents += checkEventAgainstFixtures(
					t, d.ID, channel, name, wire, ev.When, ev.Map, ev.Unverified, nil)
			}
		})
	}

	if checkedEvents == 0 {
		t.Fatal("no event was checked against a fixture; this test is not testing anything")
	}
}

// checkChannelBlockEvent walks each channel a HasChannelBlocks() event declares
// on its own, through the same channel-aware accessors production resolution
// uses, and returns how many were actually checked against a fixture.
func checkChannelBlockEvent(t *testing.T, d *spec.Descriptor, name string, ev spec.EventSpec) int {
	t.Helper()
	checked := 0
	for _, ch := range []spec.Channel{spec.ChannelAPI, spec.ChannelHooks} {
		wire, direction := ev.WireEventFor(ch)
		if wire.Empty() || direction == "out" {
			continue // this event declares no block for this channel
		}
		fields, _ := d.EventFieldsFor(name, ch)
		block := ev.API
		if ch == spec.ChannelHooks {
			block = ev.Hooks
		}
		checked += checkEventAgainstFixtures(
			t, d.ID, ch, name+"/"+string(ch), wire, ev.WhenFor(ch), fields,
			block.Unverified, block.ByWire)
	}
	return checked
}

// TestCheckEventAgainstFixtures_UnverifiedOptOutLogsRatherThanFails pins one
// side of the enforcement mechanism design spec 2.4 asks for, in isolation
// from the two real descriptors: declaring unverified: true on a block with
// zero recorded fixtures must not fail the suite, only report the gap. Its
// opposite — an UNMARKED gap failing the suite — is not expressible as a
// t.Run subtest here (a failing subtest always marks its parent failed too,
// by Go's own testing semantics, so a test cannot assert "this subtest fails"
// without itself going red); that side is instead proven directly by this
// package's own history — every one of the fixtureless blocks below FAILED
// TestV3Descriptors_ResolveAgainstRecordedTraffic until its own
// unverified: true was added (see codex.yaml/claude.yaml's own comments).
func TestCheckEventAgainstFixtures_UnverifiedOptOutLogsRatherThanFails(t *testing.T) {
	passed := t.Run("probe", func(t *testing.T) {
		checkEventAgainstFixtures(t, "probe", spec.ChannelHooks, "never_recorded",
			spec.WireRef{"NoSuchWireEventAnywhere"}, nil, spec.FieldMap{"x": {"y"}}, true, nil)
	})
	if !passed {
		t.Fatal("unverified: true must not fail the suite")
	}
}

// checkEventAgainstFixtures loads every recorded fixture for wire, unwraps it the
// way channel's own wire format expects (an api-transport frame wraps its payload
// in a JSON-RPC {"params": ...} envelope; a hooks-transport payload IS the
// envelope), and — if at least one document matches when — asserts every field in
// fields resolves against at least one of them. Returns 1 if a fixture was actually
// exercised, 0 otherwise, so the caller can require at least one real check happened.
//
// unverified is the block's own explicit opt-out (ChannelBlock.Unverified /
// EventSpec.Unverified): a declared block/event with ZERO recorded payloads is a
// hard FAILURE unless it says so itself — design spec 2.4. An ask: event that
// only fires when the CLI actually asks, or a channel genuinely never captured
// yet, marks unverified: true instead of inventing a payload or being silently
// skipped.
//
// byWire is the block's own ChannelBlock.ByWire (nil for a legacy event, or
// a channel block declaring none) — loadFixtures applies the SAME overlay
// apidriver.translateLoop applies at runtime, keyed off each fixture's own
// recorded method, so a field design spec F1 derives from the matched wire
// name is proven end to end, off the real descriptor data, not skipped.
func checkEventAgainstFixtures(
	t *testing.T,
	provider string,
	channel spec.Channel,
	label string,
	wire spec.WireRef,
	when spec.WhenMap,
	fields spec.FieldMap,
	unverified bool,
	byWire map[string]map[string]string,
) int {
	t.Helper()
	docs := loadFixtures(t, provider, channel, byWire, wire.Names()...)
	if len(docs) == 0 {
		if unverified {
			t.Logf("no fixture for %s/%s (wire %q) — declared unverified: true", provider, label, wire)
			return 0
		}
		t.Errorf("%s/%s: no recorded fixture for wire %q — every declared channel "+
			"block/event needs a real payload, or an explicit unverified: true opt-out "+
			"(design spec 2.4)", provider, label, wire)
		return 0
	}

	var matched []fixtureDoc
	for _, doc := range docs {
		if !mapping.Match(doc.params, when) {
			// A different variant of this sum-type wire event. The variant
			// that DOES match is checked by its own event.
			continue
		}
		matched = append(matched, doc)
	}
	if len(matched) == 0 {
		// A `when:`-gated event whose variant was never recorded is the exact
		// hole that let codex's `tool_result: item.content` ship: the only
		// item/started capture was a userMessage, so tool_pre and tool_post
		// were replayed against nothing and reported as fine. An unexercised
		// event is now a FAILURE, not a silent pass.
		t.Errorf("%s/%s: %d fixture(s) for wire %q but none matches its "+
			"when: %v — this event's field paths are unverified; add a "+
			"variant fixture (see testdata/fixtures/codex/README.md)",
			provider, label, len(docs), wire, when)
		return 0
	}
	assertFieldsResolve(t, provider, label, fields, matched)
	return 1
}

// assertFieldsResolve requires every mapped field to resolve against AT LEAST ONE
// of the payloads its event matches — not against all of them.
//
// The distinction matters for a sum type. codex's tool events serve five ThreadItem
// variants and the variants genuinely differ: a webSearch has no output field and no
// durationMs, an mcpToolCall reports failure in `error` rather than `result`. That is
// what the alternation in the mapping is FOR, and demanding every branch resolve in
// every variant would only push descriptor authors to stop mapping real fields.
//
// It still catches the bug this harness exists for: `tool_result: item.content`
// resolved against none of the five, because no codex item has ever had a `content`
// field.
func assertFieldsResolve(t *testing.T, provider, name string, fields spec.FieldMap, docs []fixtureDoc) {
	t.Helper()
	for field, paths := range fields {
		if len(paths) == 0 {
			continue
		}
		var tried []string
		resolved := false
		for _, doc := range docs {
			tried = append(tried, doc.name)
			if resolves(doc.params, paths) {
				resolved = true
				break
			}
		}
		if !resolved {
			t.Errorf("%s/%s: %q -> %q resolved to nothing against any recorded "+
				"payload (tried %s)", provider, name, field, paths, strings.Join(tried, ", "))
		}
	}
}

func resolves(doc map[string]any, paths []string) bool {
	if _, ok := mapping.Scalar(doc, paths); ok {
		return true
	}
	// A non-scalar leaf (an object or a list handed through whole, like tool_input)
	// is still a resolution.
	if mapping.Object(doc, paths) != nil || mapping.Objects(doc, paths) != nil {
		return true
	}
	return len(mapping.JSON(doc, paths)) > 0
}

type fixtureDoc struct {
	name   string
	params map[string]any
}

// loadFixtures finds every recording for a wire event. The base capture is named
// the way the capture script writes it — slashes become underscores — and a
// SUM-TYPE VARIANT of the same method adds a `.variant` before the extension:
//
//	item_completed.json                        the captured userMessage variant
//	item_completed.commandExecution-schema.json a second variant of the same method
//
// One wire method routinely serves several canonical events (codex's item/started
// and item/completed are both ThreadItem sum types), so a single fixture per
// method can only ever verify one of them.
func loadFixtures(
	t *testing.T, provider string, channel spec.Channel, byWire map[string]map[string]string, wires ...string,
) []fixtureDoc {
	t.Helper()
	var out []fixtureDoc
	for _, wire := range wires {
		out = append(out, loadFixturesFor(t, provider, channel, byWire, wire)...)
	}
	return out
}

// loadFixturesFor reads every recorded file for one wire name and unwraps it the
// way its OWN channel's wire format actually carries a payload: an api-transport
// frame is a JSON-RPC notification, {"method":..., "params": {...}}, so translate
// only ever sees the params object; a hooks-transport delivery has no envelope at
// all — the POSTed body IS the payload — so the decoded document is used as-is.
//
// byWire mirrors apidriver.translateLoop's own by_wire: overlay (design spec F1):
// an api-channel fixture's own "method" field selects byWire[method], merged into
// params the SAME way production does it — proving a field derived from the
// matched wire name off the real recorded capture, not skipping it.
func loadFixturesFor(
	t *testing.T, provider string, channel spec.Channel, byWire map[string]map[string]string, wire string,
) []fixtureDoc {
	t.Helper()
	base := strings.ReplaceAll(wire, "/", "_")
	matches, err := filepath.Glob(filepath.Join(fixtureRoot, provider, base+"*.json"))
	if err != nil {
		t.Fatalf("glob %s: %v", base, err)
	}

	var out []fixtureDoc
	for _, path := range matches {
		// Glob is a prefix match, so `turn_completed*` would also catch a
		// hypothetical `turn_completedSomethingElse.json`. Only the bare name
		// and `base.variant.json` are this method's.
		rest := strings.TrimSuffix(filepath.Base(path), ".json")
		if rest != base && !strings.HasPrefix(rest, base+".") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		var frame map[string]any
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		params := frame
		if channel == spec.ChannelAPI {
			params = unwrapAPIFrame(t, path, frame, byWire)
		}
		out = append(out, fixtureDoc{name: filepath.Base(path), params: params})
	}
	return out
}

// unwrapAPIFrame strips the JSON-RPC envelope — translate only ever sees params
// — and merges in the by_wire: overlay the frame's own method selects.
func unwrapAPIFrame(
	t *testing.T, path string, frame map[string]any, byWire map[string]map[string]string,
) map[string]any {
	t.Helper()
	params, ok := frame["params"].(map[string]any)
	if !ok {
		t.Fatalf("%s: no params object", path)
	}
	if method, _ := frame["method"].(string); method != "" {
		for k, v := range byWire[method] {
			params[k] = v
		}
	}
	return params
}

// An ask: event's reply is a template Crowbar writes back to the CLI verbatim, with
// {placeholders} substituted. If it is not valid JSON once the placeholders are filled,
// the provider rejects the answer at the worst possible moment — a human has decided
// and the relay is holding the gate open.
//
// This does NOT need a recorded payload, so it covers the two ask: events that have no
// fixture yet (see the t.Logf in the test above).
func TestV3Descriptors_ReplyTemplatesAreValidJSON(t *testing.T) {
	files, err := v3Files()
	if err != nil {
		t.Skipf("no v3 descriptors yet: %v", err)
	}

	// The placeholders a reply template may carry, and a stand-in that keeps the
	// document valid once substituted.
	fillers := map[string]string{
		"{reason_json}":     `"denied by a human"`,
		"{content_json}":    `{"field":"value"}`,
		"{tool_input_json}": `{"command":"ls"}`,
	}

	var checked int
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := descriptor.ParseV3(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for name, ev := range d.Events {
			// AnyWireEvent, not the flat ev.Ask: a channel-split ask event
			// (permission) names its wire method inside api:/hooks:, not at
			// the event's own top level — see spec.EventSpec.AnyWireEvent.
			if _, direction := ev.AnyWireEvent(); direction != "ask" {
				continue
			}
			if ev.Answerable != nil && !*ev.Answerable {
				if len(ev.Reply) > 0 {
					t.Errorf("%s/%s declares answerable:false but also carries reply "+
						"templates — one of the two is wrong", d.ID, name)
				}
				continue // observed, not answerable: the human answers in the terminal
			}
			if len(ev.Reply) == 0 {
				t.Errorf("%s/%s is an ask: event with no reply templates and no "+
					"answerable:false — a human's decision would reach nobody", d.ID, name)
				continue
			}
			for decision, tmpl := range ev.Reply {
				filled := tmpl
				for ph, v := range fillers {
					filled = strings.ReplaceAll(filled, ph, v)
				}
				// Only a {snake_case} token is a placeholder; a bare { is JSON.
				if left := placeholderRe.FindString(filled); left != "" {
					t.Errorf("%s/%s/%s: unrecognised placeholder %s in: %s",
						d.ID, name, decision, left, filled)
					continue
				}
				var probe any
				if err := json.Unmarshal([]byte(filled), &probe); err != nil {
					t.Errorf("%s/%s/%s: not valid JSON once filled: %v\n  %s",
						d.ID, name, decision, err, filled)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no reply template was checked; this test is not testing anything")
	}
}

// TestV3Descriptors_EveryInboundEventIsMappedOrNotEmitted enforces design spec
// 2.6: an absent canonical event used to mean both "the provider doesn't emit
// it" and "we forgot", indistinguishably. Every event a provider could itself
// FIRE (direction in/ask) must now be mapped in events: or listed in
// not_emitted:, so a gap is countable instead of invisible.
//
// Scoped to in/ask, like not_emitted: itself (see spec.Descriptor.NotEmitted's
// own doc): an OUTBOUND event is something CROWBAR sends, never something a
// provider "emits", and its absence is a capability gap already visible
// through outbound.Resolve/AnswerFor's own ok=false.
//
// This is a TEST-level check against the real shipped descriptors, not a
// rules.Apply/ParseV3-time gate: unlike required:/fixtures: (below), a small
// hand-built probe descriptor used to test one unrelated rule has no reason
// to declare the other ~20 canonical events it will never use, and gating
// every such descriptor on total vocabulary coverage would make ordinary rule
// tests fail for a reason they are not about.
func TestV3Descriptors_EveryInboundEventIsMappedOrNotEmitted(t *testing.T) {
	files, err := v3Files()
	if err != nil {
		t.Skipf("no v3 descriptors yet: %v", err)
	}
	vocab, err := schema.Load()
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := descriptor.ParseV3(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		t.Run(d.ID, func(t *testing.T) {
			listed := map[string]bool{}
			for _, name := range d.NotEmitted {
				rule, ok := vocab.Events[name]
				if !ok {
					t.Errorf("not_emitted names %q, which is not a canonical event", name)
					continue
				}
				if rule.Direction == "out" {
					t.Errorf("not_emitted names %q, an OUTBOUND event — not_emitted is for "+
						"events a PROVIDER could fire, never one Crowbar sends", name)
				}
				if listed[name] {
					t.Errorf("not_emitted names %q twice", name)
				}
				listed[name] = true
				if _, mapped := d.Events[name]; mapped {
					t.Errorf("%q is both mapped in events: and listed in not_emitted: — pick one", name)
				}
			}

			var gaps []string
			for name, rule := range vocab.Events {
				if rule.Direction == "out" || listed[name] {
					continue
				}
				if _, mapped := d.Events[name]; mapped {
					continue
				}
				// telemetry has a SECOND mechanism — the v2 telemetry.callback:/
				// probe: block — that never puts it in d.Events at all (see
				// translate/telemetry.callbackMapping's own doc on why both exist).
				if name == "telemetry" && d.Telemetry != nil {
					continue
				}
				gaps = append(gaps, name)
			}
			if len(gaps) > 0 {
				sort.Strings(gaps)
				t.Errorf("%s: canonical event(s) %v are neither mapped nor listed in not_emitted:",
					d.ID, gaps)
			}
		})
	}
}

// v3Files lists every v3 descriptor, including the experimental ones.
func v3Files() ([]string, error) {
	var out []string
	err := filepath.WalkDir("descriptors-v3", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".yaml" {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}
