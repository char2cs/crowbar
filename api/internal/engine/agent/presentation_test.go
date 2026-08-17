package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestShippedDescriptorsDeclareProviderNeutralPresentationCapabilities(t *testing.T) {
	tests := []struct {
		provider     string
		completeness agent.CatalogCompleteness
		adapter      string
	}{
		{"codex", agent.CatalogCompletenessModelVisible, agent.CatalogAdapterJSONTextSection},
		{"claude", agent.CatalogCompletenessPluginOnly, agent.CatalogAdapterJSONInventoryDetails},
	}
	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			d, err := agent.ResolveDescriptor(t.TempDir(), tt.provider)
			require.NoError(t, err)
			require.NotNil(t, d.Presentation.PromptSubmit)
			require.Equal(t, agent.PromptStrategyRestartTUI, d.Presentation.PromptSubmit.Strategy)
			require.NotNil(t, d.Presentation.SlashCatalog)
			require.Equal(t, tt.completeness, d.Presentation.SlashCatalog.Completeness)
			require.Equal(t, tt.adapter, d.Presentation.SlashCatalog.Pipeline.Adapter)
		})
	}
}

func TestPromptStepsRenderCompletedMessageAsOneLiteralArgvElement(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		for _, resume := range []bool{false, true} {
			t.Run(provider+map[bool]string{false: "/fresh", true: "/resume"}[resume], func(t *testing.T) {
				d, err := agent.ResolveDescriptor(t.TempDir(), provider)
				require.NoError(t, err)
				steps, err := d.PromptSteps(resume)
				require.NoError(t, err)

				message := "--help; $(touch /tmp/not-executed) && echo done\nsecond line"
				plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
					Tmp: t.TempDir(), Cwd: t.TempDir(), Message: message,
				}, nil, steps)
				require.NoError(t, err)
				t.Cleanup(plan.Cleanup)
				require.Equal(t, []string{"--", message}, plan.Argv[len(plan.Argv)-2:])
				require.Equal(t, 1, countValue(plan.Argv, message), "message must remain one argv token")
			})
		}
	}
}

func TestPromptEndOfOptionsMakesExactForbiddenTextDataNotAFlag(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "claude")
	require.NoError(t, err)
	steps, err := d.PromptSteps(false)
	require.NoError(t, err)
	plan, err := agent.BuildSpawnPlan(d, agent.TemplateCtx{
		Tmp: t.TempDir(), Cwd: t.TempDir(), Message: "-p",
	}, nil, steps)
	require.NoError(t, err)
	t.Cleanup(plan.Cleanup)
	require.Equal(t, []string{"--", "-p"}, plan.Argv[len(plan.Argv)-2:])
}

func TestPromptStepsReturnsDefensiveCopy(t *testing.T) {
	d, err := agent.ResolveDescriptor(t.TempDir(), "codex")
	require.NoError(t, err)
	first, err := d.PromptSteps(false)
	require.NoError(t, err)
	first[len(first)-1].Args["positional"] = "mutated"
	second, err := d.PromptSteps(false)
	require.NoError(t, err)
	require.Equal(t, "{message}", second[len(second)-1].Args["positional"])
}

func TestPromptStepsAbsentCapabilityIsTerminalOnly(t *testing.T) {
	d, err := agent.LoadDescriptor([]byte(validMinimalDescriptor))
	require.NoError(t, err)
	_, err = d.PromptSteps(false)
	require.ErrorIs(t, err, agent.ErrPromptSubmitUnsupported)
}

func TestDescriptorValidationRejectsInvalidPresentationCapabilities(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "prompt strategy",
			yaml: `presentation:
  prompt_submit:
    strategy: pipe_to_tui
    fresh: [{ pass_arg: { positional: "{message}" } }]
    resume: [{ pass_arg: { positional: "{message}" } }]
`,
			want: "unsupported strategy",
		},
		{
			name: "message missing",
			yaml: `presentation:
  prompt_submit:
    strategy: restart_tui
    fresh: [{ pass_arg: { positional: "constant" } }]
    resume: [{ pass_arg: { positional: "{message}" } }]
`,
			want: "must place {message} exactly once",
		},
		{
			name: "output bound",
			yaml: `presentation:
  slash_catalog:
    completeness: complete
    max_stdout_bytes: 4194305
    pipeline:
      adapter: json_text_section
      command: ["catalog"]
      text_path: "[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+): .+$'
      item: { label: "{name}", insert_text: "{name}" }
`,
			want: "max_stdout_bytes",
		},
		{
			name: "unknown adapter",
			yaml: `presentation:
  slash_catalog:
    completeness: complete
    pipeline:
      adapter: provider_specific_magic
      command: ["catalog"]
      item: { label: "{name}", insert_text: "{name}" }
`,
			want: "unsupported adapter",
		},
		{
			name: "dynamic primary argv",
			yaml: `presentation:
  slash_catalog:
    completeness: complete
    pipeline:
      adapter: json_text_section
      command: ["catalog", "{id}"]
      text_path: "[].text"
      start_marker: "<skills>"
      end_marker: "</skills>"
      item_pattern: '(?m)^- (?P<name>[^:]+): .+$'
      item: { label: "{name}", insert_text: "{name}" }
`,
			want: "fixed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := agent.LoadDescriptor([]byte(validMinimalDescriptor + "session:\n  resume: { arg: \"resume {id}\" }\n" + tt.yaml))
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func countValue(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
