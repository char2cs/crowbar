package descriptor_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/core/svgicon"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestRegression_NoShippedDescriptorContainsTheAlternationGlyph(t *testing.T) {
	matches, err := filepath.Glob("descriptors-v3/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "this test must actually scan something")

	for _, path := range matches {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			var root yaml.Node
			require.NoError(t, yaml.Unmarshal(data, &root))
			assertNoAlternationGlyph(t, path, &root)
		})
	}
}

func assertNoAlternationGlyph(t *testing.T, path string, n *yaml.Node) {
	t.Helper()
	if n.Kind == yaml.ScalarNode && strings.Contains(n.Value, "||") {
		t.Errorf("%s:%d: %q — the || glyph must never appear in a shipped descriptor "+
			"(first_present:/any_of:/a list instead)", path, n.Line, n.Value)
	}
	for _, c := range n.Content {
		assertNoAlternationGlyph(t, path, c)
	}
}

func TestSurfaceGatedEvents_AreReportedLoudly(t *testing.T) {
	matches, err := filepath.Glob("descriptors-v3/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "this test must actually scan something")

	var reported int
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		d, err := descriptor.ParseV0(raw)
		require.NoError(t, err)

		names := make([]string, 0, len(d.Events))
		for name := range d.Events {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			surfaces := d.EventSurfaces(name)
			if len(surfaces) == 0 {
				continue
			}
			reported++
			t.Logf("SURFACE-GATED: descriptor %q event %q ingests only on surfaces %v — "+
				"confirm this is not the sole writer of a ledger fact before relying on it",
				d.ID, name, surfaces)
		}
	}
	t.Logf("%d event(s) declare a narrower surfaces: than the default (every surface)", reported)
}

const minimal = `
id: probe
spawn:
  cmd: probe-cli
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
`

func TestLoad_AcceptsAMinimalDescriptor(t *testing.T) {
	d, err := descriptor.Load([]byte(minimal))

	require.NoError(t, err)
	assert.Equal(t, "probe", d.ID)
	assert.Equal(t, "probe-cli", d.Spawn.Cmd)
}

func TestLoad_ReadsAPromptLeadAndRefusesOneThatNamesNoContext(t *testing.T) {
	withLead := func(lead string) string {
		return `
id: leader
spawn: { cmd: leader-cli, interactive_required: true }
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    delivery: websocket
    format: jsonrpc_text
    connect: [leader-cli, ticket]
    connect_map: { url: url }
    handshake: { call: hello }
events:
  prompt:
    fresh:
      - { call: session.create, capture: { session_id: id } }
    action:
      - { call: prompt.submit, send: { text: "{text}" } }
inject:
  - at: context
    prompt_lead: "` + lead + `"
`
	}

	d, err := descriptor.Load([]byte(withLead("{context}")))
	require.NoError(t, err)
	assert.Equal(t, "{context}", d.Inject[0].PromptLead)

	_, err = descriptor.Load([]byte(withLead("a fixed line")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "never names {context}")
}

func TestLoad_RejectsMalformedYAML(t *testing.T) {
	_, err := descriptor.Load([]byte("id: [unclosed"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestLoad_RejectsADescriptorThatFailsARule(t *testing.T) {
	_, err := descriptor.Load([]byte("id: \"\"\n"))

	assert.ErrorIs(t, err, descriptor.ErrInvalid)
}

const unsafeIcon = `icon: '<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><path d="M0 0"/></svg>'` + "\n"

func TestLoad_DropsAnUnsafeIconAndKeepsTheProvider(t *testing.T) {
	d, err := descriptor.Load([]byte(minimal + unsafeIcon))

	require.NoError(t, err)
	assert.Equal(t, "probe", d.ID)
	assert.Empty(t, d.Icon)
}

func TestShippedIconsArePlainSVG(t *testing.T) {
	shipped, err := descriptor.All(context.Background(), "", nil)
	require.NoError(t, err)
	require.NotEmpty(t, shipped)
	for _, d := range shipped {
		require.NotEmptyf(t, d.Icon, "%s lost its icon", d.ID)
		assert.NoErrorf(t, svgicon.Validate(d.Icon), "%s ships an icon that is not plain SVG", d.ID)
	}
}

func TestResolve_PrefersAnOnDiskOverrideOverTheEmbeddedDefault(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "claude", minimalWithID("claude", "overridden-cli"))

	d, err := descriptor.Resolve(context.Background(), home, "claude", nil)

	require.NoError(t, err)
	assert.Equal(t, "overridden-cli", d.Spawn.Cmd,
		"a user override is the whole point of resolving from disk first")
}

func TestResolve_FallsBackToTheEmbeddedDefault(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude", nil)

	require.NoError(t, err)
	assert.Equal(t, "claude", d.Spawn.Cmd)
}

func TestRegression_ShippedDescriptorsDeclareTheirShellModeSigil(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			d, err := descriptor.Resolve(context.Background(), t.TempDir(), id, nil)

			require.NoError(t, err)
			require.NotNil(t, d.Presentation.PromptSubmit)
			require.NotNil(t, d.Presentation.PromptSubmit.LeadingSigils)
			assert.Equal(t, []string{"!"}, d.Presentation.PromptSubmit.LeadingSigils.Chars)
			assert.NotEmpty(t, d.Presentation.PromptSubmit.LeadingSigils.Escape)
		})
	}
}

func TestResolve_UnknownIDIsNotFound(t *testing.T) {
	_, err := descriptor.Resolve(context.Background(), "", "no-such-provider", nil)

	assert.ErrorIs(t, err, descriptor.ErrUnknown)
}

func TestResolve_RefusesAnIDThatIsNotABareStem(t *testing.T) {
	testCases := []string{
		"../../etc/passwd",
		"sub/claude",
		`sub\claude`,
		"..",
		"",
	}
	for _, id := range testCases {
		t.Run(id, func(t *testing.T) {
			_, err := descriptor.Resolve(context.Background(), t.TempDir(), id, nil)
			assert.ErrorIs(t, err, descriptor.ErrUnknown)
		})
	}
}

func TestResolve_RespectsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := descriptor.Resolve(ctx, "", "claude", nil)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestResolve_ABrokenOverrideIsAnError(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "claude", "id: \"\"\n")

	_, err := descriptor.Resolve(context.Background(), home, "claude", nil)

	assert.ErrorIs(t, err, descriptor.ErrInvalid,
		"asking for one provider by id must report why it is unusable")
}

func TestAll_EnumeratesTheEmbeddedSetSortedByID(t *testing.T) {
	list, err := descriptor.All(context.Background(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, descriptor.ShippedIDs(t), idsOf(list))
}

func TestAll_UnionsOnDiskIDsWithTheEmbeddedSet(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "zeta", minimalWithID("zeta", "zeta-cli"))

	list, err := descriptor.All(context.Background(), home, nil)

	require.NoError(t, err)
	assert.Equal(t, append(descriptor.ShippedIDs(t), "zeta"), idsOf(list))
}

func TestAll_ABrokenOverrideOmitsOneEntryNotTheList(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "broken", "id: \"\"\n")

	list, err := descriptor.All(context.Background(), home, nil)

	require.NoError(t, err)
	assert.Equal(t, descriptor.ShippedIDs(t), idsOf(list))
}

func TestAll_IgnoresNonYAMLAndDirectories(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "notadescriptor.yaml"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0o600))

	list, err := descriptor.All(context.Background(), home, nil)

	require.NoError(t, err)
	assert.Equal(t, descriptor.ShippedIDs(t), idsOf(list))
}

func TestAll_RespectsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := descriptor.All(ctx, "", nil)

	assert.ErrorIs(t, err, context.Canceled)
}

func TestInstalled_ReportsFalseForAnEmptyOrMissingCommand(t *testing.T) {
	assert.False(t, descriptor.Installed(""))
	assert.False(t, descriptor.Installed("crowbar-definitely-not-installed-xyz"))
}

func TestInstalled_ReportsTrueForARealExecutable(t *testing.T) {
	assert.True(t, descriptor.Installed("sh"), "sh is on PATH on every supported platform")
}

func writeOverride(t *testing.T, home, id, body string) {
	t.Helper()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o600))
}

func minimalWithID(id, cmd string) string {
	return `
id: ` + id + `
spawn:
  cmd: ` + cmd + `
  interactive_required: true
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last
runtime:
  transport: hooks
  hooks:
    format: json
`
}

func idsOf(list []*spec.Descriptor) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.ID)
	}
	return out
}

func TestResolve_ShippedCodexDeclaresItsMeasuredNotice(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex", nil)
	require.NoError(t, err)

	require.Len(t, d.TerminalNotices, 1)
	assert.Equal(t, spec.TerminalNoticeUsageLimit, d.TerminalNotices[0].Kind)
	assert.Equal(t, "■", d.TerminalNotices[0].Anchor)
	assert.Equal(t, "You've hit your usage limit", d.TerminalNotices[0].Needle)
	assert.True(t, d.TerminalNotices[0].EndsTurn)
}

func TestResolve_ShippedCodexDeclaresBothBlockingModals(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex", nil)
	require.NoError(t, err)

	needles := make([]string, 0, len(d.TerminalPrompts))
	for _, p := range d.TerminalPrompts {
		needles = append(needles, p.Needle)
	}
	assert.Contains(t, needles, "Press enter to continue")
	assert.Contains(t, needles, "Press enter to confirm or esc to go back")
}

func TestResolve_ShippedClaudeDeclaresNoNotices(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude", nil)
	require.NoError(t, err)

	assert.Empty(t, d.TerminalNotices)
}

func TestShippedClaudeDeclaresHotswapTrue(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude", nil)
	require.NoError(t, err)
	assert.True(t, d.Runtime.Hotswap, "claude keeps the PTY attached for the whole "+
		"session with hooks reporting alongside (design spec §3.5)")
}

func TestCodexDescriptor_DeclaresAttachWithoutHotswap(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex", nil)
	require.NoError(t, err)

	assert.NotEmpty(t, d.Runtime.API.Attach, "idle-only handoff needs a bare resume argv to fork")
	assert.False(t, d.Runtime.Hotswap, "codex hands its live turn over, it never shares it")
}

func TestShippedClaudeDeclaresBothSurfacesStartHere(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude", nil)
	require.NoError(t, err)

	require.Contains(t, d.Surfaces, "chat")
	require.Contains(t, d.Surfaces, "terminal")
	assert.True(t, d.Surfaces["chat"].StartHere)
	assert.True(t, d.Surfaces["terminal"].StartHere)
	assert.True(t, d.SurfaceStartHere("terminal"))
}

func TestCodexDescriptor_BothSurfacesAreStartHere(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex", nil)
	require.NoError(t, err)

	require.Contains(t, d.Surfaces, "terminal", "codex declares attach: it HAS a terminal")
	assert.Equal(t, spec.ChannelHooks, d.Surfaces["terminal"].Channel,
		"the terminal's facts arrive over the PTY's own hooks, never the api connection")
	assert.True(t, d.Surfaces["terminal"].StartHere)
	assert.True(t, d.SurfaceStartHere("terminal"))
	require.Contains(t, d.Surfaces, "chat")
	assert.Equal(t, spec.ChannelAPI, d.Surfaces["chat"].Channel)
	assert.True(t, d.Surfaces["chat"].StartHere)
}

func TestCodexDescriptor_IsMergedMixedTransport(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex", nil)
	require.NoError(t, err)

	assert.Equal(t, "api", d.Runtime.Transport)
	assert.NotEmpty(t, d.Runtime.API.Serve)
	assert.NotEmpty(t, d.Runtime.Hooks.Format, "hooks stay declared — see codex.yaml's own comment on why")

	start, _ := d.Events["subagent_pre"].WireEventFor(spec.ChannelAPI)
	assert.False(t, start.Empty(), "multi-agent v2 reports a subagent's start over api too")
	for _, name := range []string{"subagent_post", "session_end"} {
		wire, _ := d.Events[name].WireEventFor(spec.ChannelAPI)
		assert.True(t, wire.Empty(), "event %q must not be resolvable from the api channel", name)
		hooksWire, _ := d.Events[name].WireEventFor(spec.ChannelHooks)
		assert.False(t, hooksWire.Empty(), "event %q is reported over hooks", name)
	}
	apiOnly := []string{
		"session_start", "user_prompt", "turn_stop", "tool_pre", "tool_post",
		"message_delta", "permission", "elicitation", "telemetry", "interrupt", "compact_start",
		"compact_pre", "compact_post",
	}
	for _, name := range apiOnly {
		assert.Equal(t, "api", d.TransportFor(name), "event %q must be on the api default", name)
	}
}

func TestExperimentalCodexAPIDescriptorIsGone(t *testing.T) {
	_, err := os.Stat("descriptors-v3/experimental/codex-api.yaml")
	assert.True(t, os.IsNotExist(err), "codex-api.yaml is merged into codex.yaml — it must not exist alongside it")
}

func TestCodexDescriptor_FullAutoLevelUsesTheRealBypassFlag(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex", nil)
	require.NoError(t, err)

	require.NotNil(t, d.PermissionLevels)
	level, ok := d.PermissionLevels.Levels["full-auto"]
	require.True(t, ok, "codex must still declare a full-auto level")

	require.Len(t, level.Apply, 1,
		"full-auto must apply exactly the bypass flag, not stack it alongside --sandbox/--ask-for-approval")
	step := level.Apply[0]
	assert.Equal(t, "pass_arg", step.Verb)
	assert.Equal(t, "--dangerously-bypass-approvals-and-sandbox", step.Args["arg"])
	assert.NotContains(t, step.Args, "value",
		"the bypass flag is a standalone boolean flag, never given a value")

	assert.Equal(t, map[string]string{
		"sandbox":        "danger-full-access",
		"approvalPolicy": "never",
	}, level.Vars)
}
