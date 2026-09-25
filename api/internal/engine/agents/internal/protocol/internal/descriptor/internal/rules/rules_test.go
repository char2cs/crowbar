package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func valid() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Spawn.Cmd = "probe-cli"
	d.Spawn.InteractiveRequired = true
	d.Runtime.Hooks.Format = "json"
	d.Events = map[string]spec.EventSpec{
		spec.HookSessionStart: {In: spec.WireRef{"SessionStart"}, Map: spec.FieldMap{"session_id": {"session_id"}}},
		spec.HookTurnStop:     {In: spec.WireRef{"Stop"}, Map: spec.FieldMap{"message": {"last"}}},
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
			"interactive not asserted",
			func(d *spec.Descriptor) { d.Spawn.InteractiveRequired = false },
			"interactive_required",
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

func TestPromptSubmit_AcceptsTheOneStrategyThisDaemonImplements(t *testing.T) {
	require.NoError(t, rules.Apply(withPromptSubmit(valid(), spec.DeliveryRestartTUI)))
}

func TestPromptSubmit_AcceptsADeclaredLeadingSigil(t *testing.T) {
	d := withPromptSubmit(valid(), spec.DeliveryRestartTUI)
	d.Presentation.PromptSubmit.LeadingSigils = &spec.LeadingSigilsSpec{Chars: []string{"!"}, Escape: " "}
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
			"a verb other than pass_arg",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.Fresh = []spec.InjectStep{
					{Verb: "write_file", Args: map[string]any{"path": "/tmp/x", "content": "{message}"}},
				}
			},
			"may only pass argv",
		},
		{
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
			"message never placed",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.Fresh = []spec.InjectStep{
					passArg(map[string]any{"positional": "--"}),
				}
			},
			"exactly once",
		},
		{
			"leading sigils declared with no characters",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.LeadingSigils = &spec.LeadingSigilsSpec{Escape: " "}
			},
			"leading_sigils.chars is empty",
		},
		{
			"a leading sigil that is the empty string",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.LeadingSigils = &spec.LeadingSigilsSpec{
					Chars: []string{""}, Escape: " ",
				}
			},
			"holds an empty entry",
		},
		{
			"no escape to hide the sigil behind",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.LeadingSigils = &spec.LeadingSigilsSpec{Chars: []string{"!"}}
			},
			"leading_sigils.escape is required",
		},
		{
			"an escape that opens with the very sigil it must hide",
			func(d *spec.Descriptor) {
				d.Presentation.PromptSubmit.LeadingSigils = &spec.LeadingSigilsSpec{
					Chars: []string{"!"}, Escape: "!!",
				}
			},
			"starts with the sigil",
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

	require.NoError(t, rules.Apply(valid()))
}

func withAPITransport(d *spec.Descriptor) *spec.Descriptor {
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{
		Protocol: "jsonrpc2",
		Serve:    []string{"probe", "app-server", "--listen", "unix://{socket}"},
	}
	return d
}

// An api-transport spawn whose connection comes up forks NO process, so
// apply: — an argv — reaches nothing. Declaring a selection with no api
// carrier is that silent drop, made at descriptor-load time instead of at
// the third chat that quietly ran the wrong model.
func TestSelection_RejectsAnAPITransportSelectionWithNoAPICarrier(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{
			"model declares no api_apply",
			func(d *spec.Descriptor) {
				d.Effort.APIApply = []spec.InjectStep{passArg(map[string]any{"arg": "-c", "value": `e="{effort}"`})}
			},
			"model.api_apply is empty",
		},
		{
			"effort declares no api_apply",
			func(d *spec.Descriptor) {
				d.Model.APIApply = []spec.InjectStep{passArg(map[string]any{"arg": "-c", "value": `m="{model}"`})}
			},
			"effort.api_apply is empty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := withSelection(withAPITransport(valid()))
			tc.mutate(d)

			err := rules.Apply(d)

			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestSelection_AcceptsAnAPITransportSelectionThatDeclaresBothCarriers(t *testing.T) {
	d := withSelection(withAPITransport(valid()))
	d.Model.APIApply = []spec.InjectStep{passArg(map[string]any{"arg": "-c", "value": `m="{model}"`})}
	d.Effort.APIApply = []spec.InjectStep{passArg(map[string]any{"arg": "-c", "value": `e="{effort}"`})}

	assert.NoError(t, rules.Apply(d))
}

// A hooks-transport spawn always forks the vendor's own PTY, so apply: is
// carrier enough and api_apply: is meaningless there — claude must not be
// dragged into declaring one.
func TestSelection_AHooksTransportSelectionNeedsNoAPICarrier(t *testing.T) {
	assert.NoError(t, rules.Apply(withSelection(valid())))
}

func TestSelection_AcceptsAnEmptyModelCatalogue(t *testing.T) {
	d := withSelection(valid())
	d.Model.Available = nil

	require.NoError(t, rules.Apply(d))
}

func withModelDiscover(d *spec.Descriptor) *spec.Descriptor {
	d.Model = &spec.ModelSpec{
		Discover: &spec.ModelDiscoverSpec{
			Command:   []string{"debug", "models"},
			Adapter:   spec.ModelDiscoverAdapterJSON,
			ItemsPath: "models[]",
			KeepWhen:  &spec.ModelFieldMatch{Field: "visibility", Equals: "list"},
			OrderBy:   "priority",
			Item: spec.ModelItemMapping{
				ID: "{slug}", Label: "{display_name}",
				Efforts: "supported_reasoning_levels[].effort", DefaultEffort: "{default_reasoning_level}",
			},
		},
		Strategy: spec.DeliveryRestartTUI,
		Apply:    []spec.InjectStep{passArg(map[string]any{"arg": "--model", "value": "{model}"})},
	}
	return d
}

func TestModelDiscover_AcceptsAWellFormedBlock(t *testing.T) {
	require.NoError(t, rules.Apply(withModelDiscover(valid())))
}

func TestModelDiscover_RejectsAvailableAndDiscoverTogether(t *testing.T) {
	d := withModelDiscover(valid())
	d.Model.Available = []string{"gpt-6-astra"}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestModelDiscover_RejectsTheBrokenShapes(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{"no command", func(d *spec.Descriptor) { d.Model.Discover.Command = nil }, "command must be fixed non-empty argv"},
		{
			"templated command",
			func(d *spec.Descriptor) { d.Model.Discover.Command = []string{"{message}"} },
			"command must be fixed argv",
		},
		{
			"forbidden flag",
			func(d *spec.Descriptor) {
				d.Spawn.ForbidFlags = []string{"debug"}
			},
			"forbidden flag",
		},
		{"unsupported adapter", func(d *spec.Descriptor) { d.Model.Discover.Adapter = "xml" }, "unsupported adapter"},
		{"no items_path", func(d *spec.Descriptor) { d.Model.Discover.ItemsPath = "" }, "items_path is required"},
		{"no item id", func(d *spec.Descriptor) { d.Model.Discover.Item.ID = "" }, "requires id and label"},
		{"no item label", func(d *spec.Descriptor) { d.Model.Discover.Item.Label = "" }, "requires id and label"},
		{
			"keep_when with no field",
			func(d *spec.Descriptor) { d.Model.Discover.KeepWhen = &spec.ModelFieldMatch{Equals: "list"} },
			"keep_when.field is required",
		},
		{
			"default_when with no field",
			func(d *spec.Descriptor) { d.Model.Discover.DefaultWhen = &spec.ModelFieldMatch{Equals: true} },
			"default_when.field is required",
		},
		{
			"timeout above the ceiling",
			func(d *spec.Descriptor) { d.Model.Discover.TimeoutMS = spec.MaxModelDiscoverTimeoutMS + 1 },
			"timeout_ms must be between",
		},
		{
			"max_stdout_bytes above the ceiling",
			func(d *spec.Descriptor) { d.Model.Discover.MaxStdoutBytes = spec.MaxModelDiscoverMaxStdoutBytes + 1 },
			"max_stdout_bytes must be between",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := withModelDiscover(valid())
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestEffortCatalog_AcceptsEmptyAvailableWhenModelDiscoverDeclared(t *testing.T) {
	d := withModelDiscover(valid())
	d.Effort = &spec.EffortSpec{
		Strategy: spec.DeliveryRestartTUI,
		Apply:    []spec.InjectStep{passArg(map[string]any{"arg": "--effort", "value": "{effort}"})},
	}

	require.NoError(t, rules.Apply(d), "efforts come from the same probe as model.discover — no static map needed")
}

func withModelManifest(d *spec.Descriptor) *spec.Descriptor {
	d.Model = &spec.ModelSpec{
		Manifest: &spec.ModelManifestSpec{
			URL:       "https://example.com/model-manifest.json",
			ItemsPath: "providers.test.models[]",
			KeepWhen:  &spec.ModelFieldMatch{Field: "status", Equals: "current"},
			Item: spec.ModelItemMapping{
				ID: "{id}", Label: "{label}", Efforts: "efforts[]",
			},
		},
		Strategy: spec.DeliveryRestartTUI,
		Apply:    []spec.InjectStep{passArg(map[string]any{"arg": "--model", "value": "{model}"})},
	}
	return d
}

func TestModelManifest_AcceptsAWellFormedBlock(t *testing.T) {
	require.NoError(t, rules.Apply(withModelManifest(valid())))
}

func TestModelCatalog_RejectsEveryPairOfSources(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*spec.Descriptor)
	}{
		{"available + discover", func(d *spec.Descriptor) {
			d.Model = withModelDiscover(&spec.Descriptor{}).Model
			d.Model.Available = []string{"a"}
		}},
		{"available + manifest", func(d *spec.Descriptor) {
			d.Model = withModelManifest(&spec.Descriptor{}).Model
			d.Model.Available = []string{"a"}
		}},
		{"discover + manifest", func(d *spec.Descriptor) {
			d.Model = withModelDiscover(&spec.Descriptor{}).Model
			d.Model.Manifest = withModelManifest(&spec.Descriptor{}).Model.Manifest
		}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := valid()
			tc.mutate(d)

			err := rules.Apply(d)

			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), "mutually exclusive")
		})
	}
}

func TestModelManifest_RejectsTheBrokenShapes(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{"no url", func(d *spec.Descriptor) { d.Model.Manifest.URL = "" }, "must be an https URL"},
		{"non-https url", func(d *spec.Descriptor) { d.Model.Manifest.URL = "http://example.com/m.json" }, "must be an https URL"},
		{"no items_path", func(d *spec.Descriptor) { d.Model.Manifest.ItemsPath = "" }, "items_path is required"},
		{"no item id", func(d *spec.Descriptor) { d.Model.Manifest.Item.ID = "" }, "requires id and label"},
		{"no item label", func(d *spec.Descriptor) { d.Model.Manifest.Item.Label = "" }, "requires id and label"},
		{
			"keep_when with no field",
			func(d *spec.Descriptor) { d.Model.Manifest.KeepWhen = &spec.ModelFieldMatch{Equals: "current"} },
			"keep_when.field is required",
		},
		{
			"timeout above the ceiling",
			func(d *spec.Descriptor) { d.Model.Manifest.TimeoutMS = spec.MaxModelManifestTimeoutMS + 1 },
			"timeout_ms must be between",
		},
		{
			"ttl above the ceiling",
			func(d *spec.Descriptor) { d.Model.Manifest.TTLMS = spec.MaxModelManifestTTLMS + 1 },
			"ttl_ms must be between",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := withModelManifest(valid())
			tc.mutate(d)
			err := rules.Apply(d)
			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestEffortCatalog_AcceptsEmptyAvailableWhenModelManifestDeclared(t *testing.T) {
	d := withModelManifest(valid())
	d.Effort = &spec.EffortSpec{
		Strategy: spec.DeliveryRestartTUI,
		Apply:    []spec.InjectStep{passArg(map[string]any{"arg": "--effort", "value": "{effort}"})},
	}

	require.NoError(t, rules.Apply(d), "efforts come from the same manifest rows as the model list — no static map needed")
}

func TestSelection_RejectsTheBrokenShapes(t *testing.T) {
	testCases := []struct {
		name    string
		mutate  func(*spec.Descriptor)
		wantMsg string
	}{
		{
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

func TestSelection_ReportsTheSameBadKeyEveryRun(t *testing.T) {
	for range 20 {
		d := withSelection(valid())
		d.Effort.Available = map[string][]string{"a": {}, "z": {}}
		err := rules.Apply(d)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `effort.available["a"] is empty`)
	}
}

func TestApply_AcceptsDeclaredTerminalPrompts(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{
		{Kind: spec.TerminalPromptTrust, Needle: "I trust this folder"},
		{Needle: "Enter to confirm"},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAnUnknownTerminalPromptKind(t *testing.T) {
	d := valid()
	d.TerminalPrompts = []spec.TerminalPromptSpec{{Kind: "worksapce_trust", Needle: "x"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown kind")
}

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

func TestApply_DeclaringNoTerminalPromptsIsValid(t *testing.T) {
	assert.NoError(t, rules.Apply(valid()))
}

func TestApply_AcceptsADeclaredTerminalNotice(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{
		{Kind: spec.TerminalNoticeUsageLimit, Needle: "You've hit your usage limit", EndsTurn: true},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAKindlessTerminalNotice(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{{Needle: "something went wrong", EndsTurn: true}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "kind is required")
}

func TestApply_RejectsAnUnknownTerminalNoticeKind(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{{Kind: "usage_limits", Needle: "x"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown kind")
}

func TestApply_RejectsAContentFreeNoticeNeedle(t *testing.T) {
	d := valid()
	d.TerminalNotices = []spec.TerminalNoticeSpec{{Kind: spec.TerminalNoticeUsageLimit, Needle: "· ⏎ ›"}}

	require.ErrorIs(t, rules.Apply(d), rules.ErrInvalidDescriptor)
}

// A descriptor that declares only the two required events still passes every rule:
// message_delta and turn_failed are optional capabilities, not obligations.
func TestApply_ADescriptorWithOnlyTheRequiredEventsIsValid(t *testing.T) {
	require.NoError(t, rules.Apply(valid()))
}

func TestApply_DeclaringNoSurfacesIsValid(t *testing.T) {
	require.NoError(t, rules.Apply(valid()))
}

func TestApply_AcceptsDeclaredSurfaces(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceChat:     {Channel: spec.ChannelHooks},
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks},
	}

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAnUnknownSurfaceName(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{"bogus": {Channel: spec.ChannelHooks}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown surface")
}

func TestApply_RejectsAnUnknownSurfaceChannel(t *testing.T) {
	d := valid()
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceChat: {Channel: "carrier-pigeon"}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "channel")
}

func TestApply_RejectsAnAPIChannelSurfaceWhenRuntimeDeclaresNoAPITransport(t *testing.T) {
	d := valid() // no runtime.api section at all
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceChat: {Channel: spec.ChannelAPI}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "no api transport")
}

func TestApply_RejectsATerminalSurfaceWhenTheDescriptorStructurallyHasNone(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2"} // no Attach: no terminal at all
	d.Surfaces = map[string]spec.SurfaceSpec{spec.SurfaceTerminal: {Channel: spec.ChannelAPI}}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "no terminal")
}

// An api-CHANNEL terminal is the api transport's own attached view, and the
// only argv that renders it (runtime.api.attach) names a {session_id} that
// does not exist until a session has been established. Without hotswap
// nothing renders it at spawn at all (apiconn.go's applyAPITransport gate) —
// it is reached only by SwitchToTerminal, which needs a completed turn. So
// that one combination cannot serve a brand-new chat.
func TestApply_RejectsTerminalStartHereOnAnAPIChannelWithoutHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelAPI, StartHere: true},
	}

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "start_here")
}

func TestApply_AcceptsTerminalStartHereOnAnAPIChannelWithHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}
	d.Runtime.Hotswap = true
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelAPI, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}

// A hooks-CHANNEL terminal is the descriptor's own spawn.cmd PTY, live from
// the instant it forks and naming no session at all — so it is spawnable into
// from birth whether or not the descriptor hotswaps.
func TestApply_AcceptsTerminalStartHereOnAHooksChannelWithoutHotswap(t *testing.T) {
	d := valid()
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}

// codex's own shape: api transport, attach declared WITHOUT hotswap, and a
// terminal surface fed by the hooks channel. Its native TUI at birth is the
// ordinary `codex` PTY every spawn already forks, not the session-scoped
// `codex resume {id}` SwitchToTerminal needs — so start_here is legal here,
// and the idle-only attach restriction stays where it belongs, on the SWITCH.
func TestApply_AcceptsTerminalStartHereForAnAPITransportWithAHooksTerminal(t *testing.T) {
	d := valid()
	d.Runtime.Transport = "api"
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}
	d.Runtime.Hotswap = false
	d.Surfaces = map[string]spec.SurfaceSpec{
		spec.SurfaceChat:     {Channel: spec.ChannelAPI, StartHere: true},
		spec.SurfaceTerminal: {Channel: spec.ChannelHooks, StartHere: true},
	}

	assert.NoError(t, rules.Apply(d))
}

// Design spec P6b tag 2: per-event surfaces: is a spelling check only — NO
// CROWBAR-SIDE VETO on which events may gate off a surface, even the only
// writer of a ledger fact (that is reported, not rejected — see
// TestSurfaceGatedEvents_AreReportedLoudly, descriptor package).
func TestApply_DeclaringNoPerEventSurfacesIsValid(t *testing.T) {
	require.NoError(t, rules.Apply(valid()))
}

func TestApply_AcceptsKnownPerEventSurfaces(t *testing.T) {
	d := valid()
	e := d.Events[spec.HookTurnStop]
	e.Surfaces = []string{spec.SurfaceChat, spec.SurfaceTerminal}
	d.Events[spec.HookTurnStop] = e

	assert.NoError(t, rules.Apply(d))
}

func TestApply_RejectsAnUnknownPerEventSurface(t *testing.T) {
	d := valid()
	e := d.Events[spec.HookTurnStop]
	e.Surfaces = []string{"underwater"}
	d.Events[spec.HookTurnStop] = e

	err := rules.Apply(d)

	require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
	assert.Contains(t, err.Error(), "unknown surface")
}
