package descriptor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var placeholderRe = regexp.MustCompile(`\{[a-z][a-z0-9_]*\}`)

const fixtureRoot = "../protocol/testdata/fixtures"

// Every inbound field a v3 descriptor maps must resolve against RECORDED provider
// traffic. This is the only mechanism that catches a provider changing payload shape,
// and it is what closes the design spec's open question about unverified leaf paths.
//
// It has already earned its place: four paths written from the published JSON schema
// were wrong against real traffic — turn.lastAgentMessage and item.output do not
// exist, the delta carries no sequence, and tokenUsage nests under total/.
func TestV3Descriptors_ResolveAgainstRecordedTraffic(t *testing.T) {
	dir := "descriptors-v3"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no v3 descriptors yet: %v", err)
	}

	var checkedEvents int
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		d, err := descriptor.ParseV3(raw)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}

		t.Run(d.ID, func(t *testing.T) {
			for name, ev := range d.Events {
				wire, direction := ev.WireEvent()
				if direction == "out" {
					continue // an outbound call has no inbound payload to replay
				}
				doc, ok := loadFixture(t, d.ID, wire)
				if !ok {
					// An ask: event only appears when the CLI actually asks. Recording
					// one needs a permission prompt mid-turn; until that capture exists
					// the gap is REPORTED, never silently passed.
					t.Logf("no fixture for %s/%s (wire %q) — event unverified", d.ID, name, wire)
					continue
				}
				checkedEvents++

				if !mapping.Match(doc, ev.When) {
					// The recorded payload is a different variant of this wire event.
					// That is expected for sum types; the variant that DOES match is
					// checked by its own event.
					continue
				}
				assertFieldsResolve(t, d.ID, name, ev, doc)
			}
		})
	}

	if checkedEvents == 0 {
		t.Fatal("no event was checked against a fixture; this test is not testing anything")
	}
}

func assertFieldsResolve(t *testing.T, provider, name string, ev spec.EventSpec, doc map[string]any) {
	t.Helper()
	for field, expr := range ev.Map {
		if expr == "" {
			continue
		}
		if _, ok := mapping.Scalar(doc, expr); ok {
			continue
		}
		// A non-scalar leaf (an object or a list handed through whole, like
		// tool_input) is still a resolution.
		if mapping.Object(doc, expr) != nil || mapping.Objects(doc, expr) != nil {
			continue
		}
		if len(mapping.JSON(doc, expr)) > 0 {
			continue
		}
		t.Errorf("%s/%s: %q -> %q resolved to nothing against the recorded payload",
			provider, name, field, expr)
	}
}

// loadFixture finds the recording for a wire event, named the way the capture script
// writes it: slashes become underscores.
func loadFixture(t *testing.T, provider, wire string) (map[string]any, bool) {
	t.Helper()
	path := filepath.Join(fixtureRoot, provider, strings.ReplaceAll(wire, "/", "_")+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var frame map[string]any
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	// The transport unwraps the JSON-RPC envelope; translate sees params.
	params, ok := frame["params"].(map[string]any)
	if !ok {
		return nil, false
	}
	return params, true
}

// An ask: event's reply is a template Crowbar writes back to the CLI verbatim, with
// {placeholders} substituted. If it is not valid JSON once the placeholders are filled,
// the provider rejects the answer at the worst possible moment — a human has decided
// and the relay is holding the gate open.
//
// This does NOT need a recorded payload, so it covers the two ask: events that have no
// fixture yet (see the t.Logf in the test above).
func TestV3Descriptors_ReplyTemplatesAreValidJSON(t *testing.T) {
	entries, err := os.ReadDir("descriptors-v3")
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
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("descriptors-v3", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		d, err := descriptor.ParseV3(raw)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		for name, ev := range d.Events {
			if ev.Ask == "" {
				continue
			}
			if len(ev.Reply) == 0 {
				t.Errorf("%s/%s is an ask: event with no reply templates — a human's "+
					"decision would reach nobody", d.ID, name)
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
