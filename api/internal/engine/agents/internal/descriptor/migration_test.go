package descriptor_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// A v3 descriptor must map every canonical field its v2 predecessor mapped.
//
// Losing one is a SILENT capability regression: the provider keeps working, the field
// is simply never populated, and nothing fails. This test exists only while both
// shapes coexist and is deleted with the v2 files in stage 3.
func TestV3_MapsEverythingV2Mapped(t *testing.T) {
	v3dir, v2dir := "descriptors-v3", "descriptors"
	entries, err := os.ReadDir(v3dir)
	if err != nil {
		t.Skipf("no v3 descriptors: %v", err)
	}

	var compared int
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		v2path := filepath.Join(v2dir, e.Name())
		v2raw, err := os.ReadFile(v2path)
		if err != nil {
			continue // no v2 counterpart left
		}
		v2, err := descriptor.Load(v2raw)
		if err != nil {
			t.Fatalf("%s (v2): %v", e.Name(), err)
		}
		v3raw, err := os.ReadFile(filepath.Join(v3dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		v3, err := descriptor.ParseV3(v3raw)
		if err != nil {
			t.Fatalf("%s (v3): %v", e.Name(), err)
		}
		compared++

		t.Run(v2.ID, func(t *testing.T) {
			for event, v2fields := range v2.Hooks.Events {
				v3event, ok := v3.Events[event]
				if !ok {
					t.Errorf("v3 dropped the whole event %q", event)
					continue
				}
				for field := range v2fields {
					if _, ok := v3event.Map[field]; !ok {
						t.Errorf("event %q: v3 dropped field %q", event, field)
					}
				}
			}

			// A descriptor is not only its event table. Everything that is not
			// hooks/answer/telemetry carries across UNCHANGED, and dropping any of it
			// is a silent capability loss: no model picker, no slash catalog, no
			// config injection, no terminal-prompt detection.
			assertNonEventSectionsSurvive(t, v2, v3)

			// The answer: block became each ask: event's reply:. Every decision v2
			// could send must still be sendable.
			for event, ans := range v2.Answer {
				v3event, ok := v3.Events[event]
				if !ok {
					t.Errorf("v3 dropped the answerable event %q", event)
					continue
				}
				for decision := range ans.Responses {
					if _, ok := v3event.Reply[decision]; !ok {
						t.Errorf("event %q: v3 cannot send the %q decision any more", event, decision)
					}
				}
			}
		})
	}

	if compared == 0 {
		t.Skip("no descriptor has both shapes; migration comparison is done")
	}
}

// v2 spelled alternation with a comma; v3 spells it `||`. Any comma left in a v3 path
// is a mapping that will resolve to nothing at runtime and fail silently.
func TestV3_HasNoLeftoverCommaAlternation(t *testing.T) {
	entries, err := os.ReadDir("descriptors-v3")
	if err != nil {
		t.Skipf("no v3 descriptors: %v", err)
	}
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
			t.Fatal(err)
		}
		for name, ev := range d.Events {
			for field, expr := range ev.Map {
				// A comma inside a quoted LABEL is fine; one in a path is not. Labels
				// are the suggestion_label.* family, which hold prose, not paths.
				if strings.HasPrefix(field, "suggestion_label.") {
					continue
				}
				if strings.Contains(expr, ",") {
					t.Errorf("%s/%s: %q still uses comma alternation: %q",
						d.ID, name, field, expr)
				}
			}
		}
	}
}

// assertNonEventSectionsSurvive checks the parts of a descriptor that v3 does not
// restructure. They must be present in v3 exactly when they are present in v2.
func assertNonEventSectionsSurvive(t *testing.T, v2, v3 *spec.Descriptor) {
	t.Helper()
	for _, c := range []struct {
		name       string
		inV2, inV3 bool
	}{
		{"spawn.cmd", v2.Spawn.Cmd != "", v3.Spawn.Cmd != ""},
		{"spawn.args", len(v2.Spawn.Args) > 0, len(v3.Spawn.Args) > 0},
		{"session.resume", v2.Session.Resume != nil, v3.Session.Resume != nil},
		{"model", v2.Model != nil, v3.Model != nil},
		{"effort", v2.Effort != nil, v3.Effort != nil},
		{"presentation.prompt_submit", v2.Presentation.PromptSubmit != nil, v3.Presentation.PromptSubmit != nil},
		{"presentation.slash_catalog", v2.Presentation.SlashCatalog != nil, v3.Presentation.SlashCatalog != nil},
		{"config_injection", len(v2.ConfigInjection) > 0, len(v3.ConfigInjection) > 0},
		{"mcp_injection", len(v2.MCPInject) > 0, len(v3.MCPInject) > 0},
		{"context_inject", len(v2.ContextInject) > 0, len(v3.ContextInject) > 0},
		{"resume_context_inject", len(v2.ResumeContextInject) > 0, len(v3.ResumeContextInject) > 0},
		{"terminal_prompts", len(v2.TerminalPrompts) > 0, len(v3.TerminalPrompts) > 0},
		{"terminal_notices", len(v2.TerminalNotices) > 0, len(v3.TerminalNotices) > 0},
		{"injected_prompts", len(v2.InjectedPrompts) > 0, len(v3.InjectedPrompts) > 0},
	} {
		if c.inV2 && !c.inV3 {
			t.Errorf("v3 dropped %s, which v2 declares", c.name)
		}
	}
}

func sortedStrings(in []string) []string { sort.Strings(in); return in }
