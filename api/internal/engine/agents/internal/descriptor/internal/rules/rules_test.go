package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// valid returns a descriptor every rule accepts. Each test mutates only what it
// is about, so a failure names one property rather than a whole fixture.
func valid() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Spawn.Cmd = "probe-cli"
	d.Spawn.InteractiveRequired = true
	d.Hooks.Format = "json"
	d.Hooks.Events = map[string]map[string]string{
		spec.HookSessionStart: {"session_id": "session_id"},
		spec.HookTurnStop:     {"message": "last"},
	}
	return d
}

func passArg(args map[string]any) spec.InjectStep {
	return spec.InjectStep{Verb: "pass_arg", Args: args}
}

func withPromptSubmit(d *spec.Descriptor, strategy string) *spec.Descriptor {
	d.Session.Resume = &spec.ArgSpec{Arg: "--resume {id}"}
	d.Presentation.PromptSubmit = &spec.PromptSubmitSpec{
		Strategy: strategy,
		Fresh:    []spec.InjectStep{passArg(map[string]any{"positional": "{message}"})},
		Resume:   []spec.InjectStep{passArg(map[string]any{"positional": "{message}"})},
	}
	return d
}

func withCatalog(d *spec.Descriptor) *spec.Descriptor {
	d.Presentation.SlashCatalog = &spec.SlashCatalogSpec{
		Completeness: spec.CatalogCompletenessComplete,
		Pipeline: spec.CatalogPipelineSpec{
			Adapter:     spec.CatalogAdapterJSONTextSection,
			Command:     []string{"debug", "prompt"},
			TextPath:    "content",
			StartMarker: "<s>",
			EndMarker:   "</s>",
			ItemPattern: `- (?P<name>\S+)`,
			Item:        spec.CatalogItemMapping{Label: "{name}", InsertText: "/{name} "},
		},
	}
	return d
}

func TestApply_AcceptsAValidDescriptor(t *testing.T) {
	require.NoError(t, rules.Apply(valid()))
}

func TestApply_NilDescriptorIsInvalid(t *testing.T) {
	assert.ErrorIs(t, rules.Apply(nil), rules.ErrInvalidDescriptor)
}

func TestAll_EveryRuleIsNamed(t *testing.T) {
	seen := map[string]struct{}{}
	for _, r := range rules.All() {
		require.NotEmpty(t, r.Name())
		_, dup := seen[r.Name()]
		assert.False(t, dup, "rule names must be unique: %s", r.Name())
		seen[r.Name()] = struct{}{}
	}
	assert.NotEmpty(t, seen)
}

func TestIdentityAndSpawn_RejectTheUnusableCases(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{"missing id", func(d *spec.Descriptor) { d.ID = "" }, "missing id"},
		{"missing spawn.cmd", func(d *spec.Descriptor) { d.Spawn.Cmd = "" }, "missing spawn.cmd"},
		{
			// Crowbar hosts the ordinary interactive CLI in a real PTY and never a
			// headless one. A descriptor that does not assert that is refused.
			"interactive not asserted",
			func(d *spec.Descriptor) { d.Spawn.InteractiveRequired = false },
			"interactive_required",
		},
		{"missing hooks.format", func(d *spec.Descriptor) { d.Hooks.Format = "" }, "hooks.format"},
		{
			"session_start does not map session_id",
			func(d *spec.Descriptor) { d.Hooks.Events[spec.HookSessionStart] = map[string]string{} },
			"session_start must map session_id",
		},
		{
			"turn_stop does not map message",
			func(d *spec.Descriptor) { d.Hooks.Events[spec.HookTurnStop] = map[string]string{} },
			"turn_stop must map message",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestPromptSubmit_AcceptsBothShippedStrategies(t *testing.T) {
	require.NoError(t, rules.Apply(withPromptSubmit(valid(), spec.DeliveryRestartTUI)))

	d := withPromptSubmit(valid(), spec.DeliveryRewakeHook)
	d.Presentation.PromptSubmit.Rewake = &spec.RewakeSpec{Sentinel: "crowbar-delivered"}
	require.NoError(t, rules.Apply(d))
}

func TestPromptSubmit_RejectsTheBrokenShapes(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{
			"unknown strategy",
			func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Strategy = "telepathy" },
			"unsupported strategy",
		},
		{
			// rewake is an optimisation over a channel that can move; without its
			// discriminator Crowbar cannot tell its own delivery from a real one.
			"rewake without a sentinel",
			func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Strategy = spec.DeliveryRewakeHook },
			"requires rewake.sentinel",
		},
		{
			// Every strategy falls back to a fresh spawn for message #1.
			"no resume argument",
			func(d *spec.Descriptor) { d.Session.Resume = nil },
			"requires session.resume",
		},
		{
			"empty fresh steps",
			func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Fresh = nil },
			"fresh is empty",
		},
		{
			"empty resume steps",
			func(d *spec.Descriptor) { d.Presentation.PromptSubmit.Resume = nil },
			"resume is empty",
		},
		{
			// A prompt reaches the CLI as argv and nothing else. A write_file here
			// would put the user's message on disk.
			"a verb other than pass_arg",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.Fresh = []spec.InjectStep{
					{Verb: "write_file", Args: map[string]any{"path": "/tmp/x", "content": "{message}"}},
				}
			},
			"may only pass argv",
		},
		{
			// Twice delivers the prompt twice.
			"message placed twice",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.Fresh = []spec.InjectStep{
					passArg(map[string]any{"positional": "{message}"}),
					passArg(map[string]any{"positional": "{message}"}),
				}
			},
			"exactly once",
		},
		{
			// Never spawns a CLI that silently drops what the user typed.
			"message never placed",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.Fresh = []spec.InjectStep{
					passArg(map[string]any{"positional": "--"}),
				}
			},
			"exactly once",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := withPromptSubmit(valid(), spec.DeliveryRestartTUI)
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestCatalog_AcceptsBothAdapters(t *testing.T) {
	require.NoError(t, rules.Apply(withCatalog(valid())))

	d := withCatalog(valid())
	d.Presentation.SlashCatalog.Completeness = spec.CatalogCompletenessPluginOnly
	d.Presentation.SlashCatalog.Pipeline = spec.CatalogPipelineSpec{
		Adapter:            spec.CatalogAdapterJSONInventoryDetails,
		Command:            []string{"plugin", "list", "--json"},
		RowsPath:           "[]",
		EnabledField:       "enabled",
		IDField:            "id",
		DetailCommand:      []string{"plugin", "details", "{id}"},
		DetailPattern:      `Skills \((?P<items>[^)]*)\)`,
		DetailItemsGroup:   "items",
		DetailSeparator:    ",",
		DetailEmptyPattern: `Skills \(0\)`,
		SourcePattern:      `^(?P<source>[^@]+)`,
		Item:               spec.CatalogItemMapping{Label: "{source}:{name}", InsertText: "/{name} "},
	}
	require.NoError(t, rules.Apply(d))
}

func TestCatalog_RejectsTheBrokenShapes(t *testing.T) {
	inventory := func() spec.CatalogPipelineSpec {
		return spec.CatalogPipelineSpec{
			Adapter:          spec.CatalogAdapterJSONInventoryDetails,
			Command:          []string{"plugin", "list"},
			RowsPath:         "[]",
			EnabledField:     "enabled",
			IDField:          "id",
			DetailCommand:    []string{"plugin", "details", "{id}"},
			DetailPattern:    `(?P<items>x)`,
			DetailItemsGroup: "items",
			DetailSeparator:  ",",
			Item:             spec.CatalogItemMapping{Label: "{name}", InsertText: "/{name} "},
		}
	}
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{
			"unknown completeness",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Completeness = "probably" },
			"unsupported completeness",
		},
		{
			// The ceilings are the point: a descriptor may ask for LESS exposure to
			// a provider command and never for more.
			"timeout above the ceiling",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.TimeoutMS = spec.MaxCatalogTimeoutMS + 1 },
			"timeout_ms must be between",
		},
		{
			"negative item cap",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.MaxItems = -1 },
			"max_items must be between",
		},
		{
			"concurrency above the ceiling",
			func(d *spec.Descriptor) {
				d.Presentation.SlashCatalog.Pipeline.DetailConcurrency = spec.MaxCatalogDetailConcurrency + 1
			},
			"detail_concurrency must be between",
		},
		{
			"empty command",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Command = nil },
			"must be fixed non-empty argv",
		},
		{
			"command with an empty entry",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Command = []string{"debug", ""} },
			"must be fixed non-empty argv",
		},
		{
			// A template here is the one place a descriptor could smuggle
			// caller-controlled data into a real subprocess argv.
			"templated command",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Command = []string{"{message}"} },
			"must be fixed argv",
		},
		{
			"command carrying a forbidden flag",
			func(d *spec.Descriptor) {
				d.Spawn.ForbidFlags = []string{"--print"}
				d.Presentation.SlashCatalog.Pipeline.Command = []string{"--print"}
			},
			"forbidden flag",
		},
		{
			"item mapping with no label",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Item.Label = "" },
			"requires label and insert_text",
		},
		{
			"item mapping with no insert text",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Item.InsertText = "" },
			"requires label and insert_text",
		},
		{
			// An unrecognised placeholder survives expansion and is shown verbatim.
			"item mapping with an unknown placeholder",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Item.Label = "{unknown}" },
			"unsupported template",
		},
		{
			"unknown adapter",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.Adapter = "telepathy" },
			"unsupported adapter",
		},
		{
			"text_section missing a field",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.TextPath = "" },
			"json_text_section pipeline is incomplete",
		},
		{
			"text_section path traversal",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.TextPath = "a..b" },
			"text_path is invalid",
		},
		{
			"text_section item pattern without the name group",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.ItemPattern = `- (\S+)` },
			"named group",
		},
		{
			"text_section item pattern that does not compile",
			func(d *spec.Descriptor) { d.Presentation.SlashCatalog.Pipeline.ItemPattern = `(?P<name>[` },
			"invalid regex",
		},
		{
			"inventory missing a field",
			func(d *spec.Descriptor) {
				p := inventory()
				p.RowsPath = ""
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"json_inventory_text_detail pipeline is incomplete",
		},
		{
			"inventory detail command without an id slot",
			func(d *spec.Descriptor) {
				p := inventory()
				p.DetailCommand = []string{"plugin", "details"}
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"exactly once",
		},
		{
			"inventory detail command with an extra template",
			func(d *spec.Descriptor) {
				p := inventory()
				p.DetailCommand = []string{"plugin", "{message}", "{id}"}
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"unsupported template",
		},
		{
			"inventory detail command carrying a forbidden flag",
			func(d *spec.Descriptor) {
				d.Spawn.ForbidFlags = []string{"-p"}
				p := inventory()
				p.DetailCommand = []string{"-p", "{id}"}
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"detail_command contains forbidden flag",
		},
		{
			"inventory detail pattern missing its items group",
			func(d *spec.Descriptor) {
				p := inventory()
				p.DetailPattern = `(?P<other>x)`
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"named group",
		},
		{
			"inventory empty pattern that does not compile",
			func(d *spec.Descriptor) {
				p := inventory()
				p.DetailEmptyPattern = `(`
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"detail_empty_pattern",
		},
		{
			"inventory source pattern without the source group",
			func(d *spec.Descriptor) {
				p := inventory()
				p.SourcePattern = `^([^@]+)`
				d.Presentation.SlashCatalog.Pipeline = p
			},
			"source_pattern",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := withCatalog(valid())
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestTelemetry_AcceptsBothTransports(t *testing.T) {
	d := valid()
	d.Telemetry = &spec.TelemetrySpec{
		Callback: &spec.TelemetryCallbackSpec{
			Format: "json",
			Fields: map[string]string{spec.FactContextUsedPercent: "ctx.pct"},
			RateLimits: []spec.TelemetryRateLimitMap{
				{ID: "five_hour", UsedPercent: "rl.five.pct"},
			},
		},
		Probe: &spec.TelemetryProbeSpec{
			Format:  "json",
			Command: []string{"debug", "models"},
			Fields:  map[string]string{spec.FactContextCapacityTokens: "models.0.context_window"},
		},
	}
	require.NoError(t, rules.Apply(d))
}

func TestTelemetry_RejectsTheBrokenShapes(t *testing.T) {
	base := func() *spec.TelemetrySpec {
		return &spec.TelemetrySpec{Callback: &spec.TelemetryCallbackSpec{
			Format: "json",
			Fields: map[string]string{spec.FactContextUsedPercent: "ctx.pct"},
		}}
	}
	testCases := []struct {
		name    string
		mutate  func(*spec.TelemetrySpec)
		wantMsg string
	}{
		{"no transport", func(s *spec.TelemetrySpec) { s.Callback = nil }, "declares no transport"},
		{
			"callback format",
			func(s *spec.TelemetrySpec) { s.Callback.Format = "toml" },
			"callback has unsupported format",
		},
		{"callback maps nothing", func(s *spec.TelemetrySpec) { s.Callback.Fields = nil }, "maps no fields"},
		{
			// A closed vocabulary is what stops a typo'd key being silently ignored
			// and the gauge it was meant to fill staying blank with no explanation.
			"callback maps an unknown fact",
			func(s *spec.TelemetrySpec) { s.Callback.Fields = map[string]string{"context.vibes": "x"} },
			"unknown fact",
		},
		{
			"callback fact with an empty path",
			func(s *spec.TelemetrySpec) { s.Callback.Fields[spec.FactCostTotalUSD] = "" },
			"empty path",
		},
		{
			"rate-limit window with no id",
			func(s *spec.TelemetrySpec) {
				s.Callback.RateLimits = []spec.TelemetryRateLimitMap{{UsedPercent: "x"}}
			},
			"entry has no id",
		},
		{
			"duplicate rate-limit window",
			func(s *spec.TelemetrySpec) {
				s.Callback.RateLimits = []spec.TelemetryRateLimitMap{
					{ID: "w", UsedPercent: "x"}, {ID: "w", UsedPercent: "y"},
				}
			},
			"duplicate id",
		},
		{
			"rate-limit window mapping nothing",
			func(s *spec.TelemetrySpec) { s.Callback.RateLimits = []spec.TelemetryRateLimitMap{{ID: "w"}} },
			"maps nothing",
		},
		{
			"probe format",
			func(s *spec.TelemetrySpec) {
				s.Probe = &spec.TelemetryProbeSpec{Format: "toml", Command: []string{"x"}}
			},
			"probe has unsupported format",
		},
		{
			"probe with no command",
			func(s *spec.TelemetrySpec) {
				s.Probe = &spec.TelemetryProbeSpec{Format: "json"}
			},
			"command must be fixed non-empty argv",
		},
		{
			"templated probe command",
			func(s *spec.TelemetrySpec) {
				s.Probe = &spec.TelemetryProbeSpec{Format: "json", Command: []string{"{id}"}}
			},
			"must be fixed argv",
		},
		{
			"probe maps nothing",
			func(s *spec.TelemetrySpec) {
				s.Probe = &spec.TelemetryProbeSpec{Format: "json", Command: []string{"debug"}}
			},
			"maps no fields",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			d.Telemetry = base()
			tc.mutate(d.Telemetry)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestTelemetry_ProbeCommandCarryingAForbiddenFlagIsRejected(t *testing.T) {
	d := valid()
	d.Spawn.ForbidFlags = []string{"exec"}
	d.Telemetry = &spec.TelemetrySpec{Probe: &spec.TelemetryProbeSpec{
		Format:  "json",
		Command: []string{"exec"},
		Fields:  map[string]string{spec.FactModelID: "m"},
	}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "forbidden flag")
}

// withSelection gives a descriptor both selection blocks in the shape claude.yaml
// declares them.
func withSelection(d *spec.Descriptor) *spec.Descriptor {
	d.Model = &spec.ModelSpec{
		Available: []string{"sonnet", "opus"},
		Strategy:  spec.DeliveryRestartTUI,
		Apply:     []spec.InjectStep{passArg(map[string]any{"arg": "--model", "value": "{model}"})},
	}
	d.Effort = &spec.EffortSpec{
		Available: map[string][]string{spec.EffortFallbackKey: {"low", "high"}},
		Strategy:  spec.DeliveryRestartTUI,
		Apply:     []spec.InjectStep{passArg(map[string]any{"arg": "--effort", "value": "{effort}"})},
	}
	return d
}

func TestSelection_AcceptsBothBlocksAndTheirAbsence(t *testing.T) {
	require.NoError(t, rules.Apply(withSelection(valid())))
	// Absent is the codex shape and must stay valid: no picker is a capability
	// statement, not a broken descriptor.
	require.NoError(t, rules.Apply(valid()))
}

// TestSelection_AcceptsAnEmptyModelCatalogue keeps "declared but not yet
// populated" legal. An empty LIST is a picker with nothing in it, which is
// legible; an empty ENTRY is not (see the rejection table below).
func TestSelection_AcceptsAnEmptyModelCatalogue(t *testing.T) {
	d := withSelection(valid())
	d.Model.Available = nil

	require.NoError(t, rules.Apply(d))
}

func TestSelection_RejectsTheBrokenShapes(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{
			// The only strategy that exists. A live switch cannot be tested against
			// either CLI, so a descriptor asking for one must fail rather than leave
			// a picker that silently does nothing.
			"model strategy is not restart_tui",
			func(d *spec.Descriptor) { d.Model.Strategy = "live_switch" },
			"model.strategy must be",
		},
		{
			"effort strategy is not restart_tui",
			func(d *spec.Descriptor) { d.Effort.Strategy = "" },
			"effort.strategy must be",
		},
		{
			// A block with no apply advertises a capability, accepts a choice and
			// delivers it nowhere.
			"model declares no apply",
			func(d *spec.Descriptor) { d.Model.Apply = nil },
			"model.apply is empty",
		},
		{
			"effort declares no apply",
			func(d *spec.Descriptor) { d.Effort.Apply = nil },
			"effort.apply is empty",
		},
		{
			// An empty id renders as a blank row and, if chosen, as an empty argv
			// value behind the flag — where the next token becomes the flag's value.
			"model catalogue holds an empty id",
			func(d *spec.Descriptor) { d.Model.Available = []string{"sonnet", ""} },
			"model.available[1] is empty",
		},
		{
			"effort catalogue keys nothing",
			func(d *spec.Descriptor) { d.Effort.Available = map[string][]string{} },
			"effort.available declares no models",
		},
		{
			"effort catalogue holds an empty list",
			func(d *spec.Descriptor) { d.Effort.Available = map[string][]string{"opus": {}} },
			`effort.available["opus"] is empty`,
		},
		{
			"effort catalogue holds an empty level",
			func(d *spec.Descriptor) { d.Effort.Available = map[string][]string{"opus": {"low", ""}} },
			`effort.available["opus"][1] is empty`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := withSelection(valid())
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

// TestSelection_ReportsTheSameBadKeyEveryRun pins the sorted iteration. Go
// randomises map order, so a descriptor with two bad keys would otherwise blame a
// different one between runs and make the failure unreproducible.
func TestSelection_ReportsTheSameBadKeyEveryRun(t *testing.T) {
	for range 20 {
		d := withSelection(valid())
		d.Effort.Available = map[string][]string{"a": {}, "z": {}}
		err := rules.Apply(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `effort.available["a"] is empty`)
	}
}

// --- terminal_prompts ---

// TestApply_AcceptsDeclaredTerminalPrompts covers both shapes a descriptor may
// declare: a needle that identifies a prompt specifically, and one that only
// proves a modal is up.
func TestApply_AcceptsDeclaredTerminalPrompts(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{
		{Kind: spec.TerminalPromptTrust, Needle: "I trust this folder"},
		{Needle: "Enter to confirm"},
	}

	assert.NoError(t, rules.Apply(d))
}

// TestApply_RejectsAnUnknownTerminalPromptKind is why a typo cannot ship. A kind
// the daemon does not know would silently degrade to the generic case and look
// like a working descriptor forever — and the one thing this feature exists to
// prevent is a state that explains nothing.
func TestApply_RejectsAnUnknownTerminalPromptKind(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Kind: "worksapce_trust", Needle: "x"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown kind")
}

// TestApply_RejectsAContentFreeNeedle: a needle of pure punctuation reduces to the
// empty string under the matcher's own comparison, which every screen contains —
// so it would report every idle chat as blocked.
func TestApply_RejectsAContentFreeNeedle(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Needle: "· ⏎ ›"}}

	require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
}

func TestApply_RejectsAnEmptyNeedle(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Kind: spec.TerminalPromptTrust}}

	require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
}

// TestApply_DeclaringNoTerminalPromptsIsValid pins the default. Most providers
// will declare none, and that must never be a validation failure.
func TestApply_DeclaringNoTerminalPromptsIsValid(t *testing.T) {
	assert.NoError(t, rules.Apply(valid()))
}
