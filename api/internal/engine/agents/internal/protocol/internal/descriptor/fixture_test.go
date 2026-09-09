package descriptor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
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
				wire, direction := ev.WireEvent()
				if direction == "out" {
					continue // an outbound call has no inbound payload to replay
				}
				docs := loadFixtures(t, d.ID, wire)
				if len(docs) == 0 {
					// An ask: event only appears when the CLI actually asks. Recording
					// one needs a permission prompt mid-turn; until that capture exists
					// the gap is REPORTED, never silently passed.
					t.Logf("no fixture for %s/%s (wire %q) — event unverified", d.ID, name, wire)
					continue
				}

				var matched []fixtureDoc
				for _, doc := range docs {
					if !mapping.Match(doc.params, ev.When) {
						// A different variant of this sum-type wire event. The variant
						// that DOES match is checked by its own event.
						continue
					}
					matched = append(matched, doc)
				}
				if len(matched) > 0 {
					checkedEvents++
					assertFieldsResolve(t, d.ID, name, ev, matched)
				}

				// A `when:`-gated event whose variant was never recorded is the exact
				// hole that let codex's `tool_result: item.content` ship: the only
				// item/started capture was a userMessage, so tool_pre and tool_post
				// were replayed against nothing and reported as fine. An unexercised
				// event is now a FAILURE, not a silent pass.
				if len(matched) == 0 {
					t.Errorf("%s/%s: %d fixture(s) for wire %q but none matches its "+
						"when: %v — this event's field paths are unverified; add a "+
						"variant fixture (see testdata/fixtures/codex/README.md)",
						d.ID, name, len(docs), wire, ev.When)
				}
			}
		})
	}

	if checkedEvents == 0 {
		t.Fatal("no event was checked against a fixture; this test is not testing anything")
	}
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
func assertFieldsResolve(t *testing.T, provider, name string, ev spec.EventSpec, docs []fixtureDoc) {
	t.Helper()
	for field, expr := range ev.Map {
		if expr == "" {
			continue
		}
		var tried []string
		resolved := false
		for _, doc := range docs {
			tried = append(tried, doc.name)
			if resolves(doc.params, expr) {
				resolved = true
				break
			}
		}
		if !resolved {
			t.Errorf("%s/%s: %q -> %q resolved to nothing against any recorded "+
				"payload (tried %s)", provider, name, field, expr, strings.Join(tried, ", "))
		}
	}
}

func resolves(doc map[string]any, expr string) bool {
	if _, ok := mapping.Scalar(doc, expr); ok {
		return true
	}
	// A non-scalar leaf (an object or a list handed through whole, like tool_input)
	// is still a resolution.
	if mapping.Object(doc, expr) != nil || mapping.Objects(doc, expr) != nil {
		return true
	}
	return len(mapping.JSON(doc, expr)) > 0
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
func loadFixtures(t *testing.T, provider, wire string) []fixtureDoc {
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
		// The transport unwraps the JSON-RPC envelope; translate sees params.
		params, ok := frame["params"].(map[string]any)
		if !ok {
			t.Fatalf("%s: no params object", path)
		}
		out = append(out, fixtureDoc{name: filepath.Base(path), params: params})
	}
	return out
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
			if ev.Ask == "" {
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
