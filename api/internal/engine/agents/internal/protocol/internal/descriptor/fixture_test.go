package descriptor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

func TestV3Descriptors_ResolveAgainstRecordedTraffic(t *testing.T) {
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
		d, err := descriptor.ParseV0(raw)
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
					continue
				}
				channel := spec.ChannelAPI
				if d.TransportFor(name) == "hooks" {
					channel = spec.ChannelHooks
				}
				checkedEvents += checkEventAgainstFixtures(
					t, d.ID, channel, name, wire, ev.When, ev.Map, ev.Unverified, ev.UnverifiedFields, nil)
			}
		})
	}

	if checkedEvents == 0 {
		t.Fatal("no event was checked against a fixture; this test is not testing anything")
	}
}

func checkChannelBlockEvent(t *testing.T, d *spec.Descriptor, name string, ev spec.EventSpec) int {
	t.Helper()
	checked := 0
	for _, ch := range []spec.Channel{spec.ChannelAPI, spec.ChannelHooks} {
		wire, direction := ev.WireEventFor(ch)
		if wire.Empty() || direction == "out" {
			continue
		}
		fields, _ := d.EventFieldsFor(name, ch)
		block := ev.API
		if ch == spec.ChannelHooks {
			block = ev.Hooks
		}
		checked += checkEventAgainstFixtures(
			t, d.ID, ch, name+"/"+string(ch), wire, ev.WhenFor(ch), fields,
			block.Unverified, block.UnverifiedFields, block.ByWire)
	}
	return checked
}

func TestCheckEventAgainstFixtures_UnverifiedOptOutLogsRatherThanFails(t *testing.T) {
	passed := t.Run("probe", func(t *testing.T) {
		checkEventAgainstFixtures(t, "probe", spec.ChannelHooks, "never_recorded",
			spec.WireRef{"NoSuchWireEventAnywhere"}, nil, spec.FieldMap{"x": mapping.Paths("y")}, true, nil, nil)
	})
	if !passed {
		t.Fatal("unverified: true must not fail the suite")
	}
}

func checkEventAgainstFixtures(
	t *testing.T,
	provider string,
	channel spec.Channel,
	label string,
	wire spec.WireRef,
	when spec.WhenMap,
	fields spec.FieldMap,
	unverified bool,
	unproven []string,
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
			continue
		}
		matched = append(matched, doc)
	}
	if len(matched) == 0 {
		t.Errorf("%s/%s: %d fixture(s) for wire %q but none matches its "+
			"when: %v — this event's field paths are unverified; add a "+
			"variant fixture (see testdata/fixtures/codex/README.md)",
			provider, label, len(docs), wire, when)
		return 0
	}
	assertFieldsResolve(t, provider, label, fields, matched, unproven)
	return 1
}

func assertFieldsResolve(
	t *testing.T, provider, name string, fields spec.FieldMap, docs []fixtureDoc, unproven []string,
) {
	t.Helper()
	for field, paths := range fields {
		if len(paths) == 0 || isSubObjectField(field) {
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
		switch isUnproven := slices.Contains(unproven, field); {
		case resolved && isUnproven:
			t.Errorf("%s/%s: %q resolves now; drop it from unverified_fields", provider, name, field)
		case !resolved && isUnproven:
			t.Logf("%s/%s: %q resolves against no recording — declared in unverified_fields", provider, name, field)
		case !resolved:
			t.Errorf("%s/%s: %q resolved to nothing against any recorded "+
				"payload (tried %s)", provider, name, field, strings.Join(tried, ", "))
		}
	}
	for _, field := range unproven {
		if _, mapped := fields[field]; !mapped {
			t.Errorf("%s/%s: unverified_fields names %q, which this block does not map", provider, name, field)
		}
	}
}

func isSubObjectField(field string) bool {
	return strings.HasPrefix(field, "suggestion_") || strings.HasPrefix(field, "question_") ||
		strings.HasPrefix(field, "option_")
}

func resolves(doc map[string]any, paths mapping.Expr) bool {
	if _, ok := mapping.Scalar(doc, paths); ok {
		return true
	}
	if mapping.Object(doc, paths) != nil || mapping.Objects(doc, paths) != nil {
		return true
	}
	return len(mapping.JSON(doc, paths)) > 0
}

type fixtureDoc struct {
	name   string
	params map[string]any
}

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

func TestV3Descriptors_ReplyTemplatesAreValidJSON(t *testing.T) {
	files, err := v3Files()
	if err != nil {
		t.Skipf("no v3 descriptors yet: %v", err)
	}

	fillers := map[string]string{
		"{answers_json}":     `{"q0":"yes"}`,
		"{reason_json}":      `"denied by a human"`,
		"{content_json}":     `{"field":"value"}`,
		"{tool_input_json}":  `{"command":"ls"}`,
		"{suggestions_json}": `[{"type":"setMode","mode":"acceptEdits"}]`,
		"{suggestion_json}":  `{"type":"setMode","mode":"acceptEdits"}`,
	}

	var checked int
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := descriptor.ParseV0(raw)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		for name, ev := range d.Events {
			if _, direction := ev.AnyWireEvent(); direction != "ask" {
				continue
			}
			if ev.Answerable != nil && !*ev.Answerable {
				if len(ev.Reply) > 0 {
					t.Errorf("%s/%s declares answerable:false but also carries reply "+
						"templates — one of the two is wrong", d.ID, name)
				}
				continue
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
		d, err := descriptor.ParseV0(raw)
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
