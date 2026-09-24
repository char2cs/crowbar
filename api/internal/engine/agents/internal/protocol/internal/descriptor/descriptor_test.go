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

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// TestRegression_NoShippedDescriptorContainsTheAlternationGlyph is the repo-
// level backstop for docs/plans/2026-09-22-descriptor-channel-split.md 5.0: by
// the end of P5 the `||` glyph must not appear in ANY descriptor, in ANY
// position. The parser (spec.decodeExpr/WireRef.UnmarshalYAML) already
// refuses it in an expression, but this scans the raw YAML NODE tree — every
// scalar, key or value, anywhere in the document — so a reintroduction
// outside a field this phase's parser rule happens to cover (or in a brand
// new descriptor nobody wired the same check into) still fails the build.
//
// Walking yaml.Node rather than the raw text is what makes "outside a
// comment" free: a YAML comment is never captured into a Node's Value at all,
// so there is nothing to strip.
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

// TestSurfaceGatedEvents_AreReportedLoudly is the conformance layer's own
// visibility for design spec P6b's "visibility, not veto": a descriptor may
// gate ANY event off ANY surface — there is no Crowbar-side policy veto, the
// descriptor author decides (P6b, "NO CROWBAR-SIDE VETO") — but the
// implication must be LOUD and greppable, the same as unverified: true. This
// test never fails on what it finds; it only reports, the same way `grep
// surfaces: descriptors-v3/*.yaml` would, but tied to the real parsed table
// rather than a raw text match.
func TestSurfaceGatedEvents_AreReportedLoudly(t *testing.T) {
	matches, err := filepath.Glob("descriptors-v3/*.yaml")
	require.NoError(t, err)
	require.NotEmpty(t, matches, "this test must actually scan something")

	var reported int
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		d, err := descriptor.ParseV3(raw)
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

func TestLoad_RejectsMalformedYAML(t *testing.T) {
	_, err := descriptor.Load([]byte("id: [unclosed"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestLoad_RejectsADescriptorThatFailsARule(t *testing.T) {
	_, err := descriptor.Load([]byte("id: \"\"\n"))

	assert.ErrorIs(t, err, descriptor.ErrInvalid)
}

func TestResolve_PrefersAnOnDiskOverrideOverTheEmbeddedDefault(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "claude", minimalWithID("claude", "overridden-cli"))

	d, err := descriptor.Resolve(context.Background(), home, "claude")

	require.NoError(t, err)
	assert.Equal(t, "overridden-cli", d.Spawn.Cmd,
		"a user override is the whole point of resolving from disk first")
}

func TestResolve_FallsBackToTheEmbeddedDefault(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude")

	require.NoError(t, err)
	assert.Equal(t, "claude", d.Spawn.Cmd)
}

// REGRESSION. Both shipped CLIs open a shell mode on a message whose FIRST
// character is `!` — and Crowbar's own image-attachment encoding, `![alt](…)`,
// puts one there whenever something is attached before anything is typed.
// Measured on codex-cli 0.149.1: the message was RUN instead of sent. The
// dispatch-time guard is driven entirely off this declaration, so a descriptor
// that stops declaring it silently re-opens the hole.
func TestRegression_ShippedDescriptorsDeclareTheirShellModeSigil(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			d, err := descriptor.Resolve(context.Background(), t.TempDir(), id)

			require.NoError(t, err)
			require.NotNil(t, d.Presentation.PromptSubmit)
			require.NotNil(t, d.Presentation.PromptSubmit.LeadingSigils)
			assert.Equal(t, []string{"!"}, d.Presentation.PromptSubmit.LeadingSigils.Chars)
			assert.NotEmpty(t, d.Presentation.PromptSubmit.LeadingSigils.Escape)
		})
	}
}

func TestResolve_UnknownIDIsNotFound(t *testing.T) {
	_, err := descriptor.Resolve(context.Background(), "", "no-such-provider")

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
			_, err := descriptor.Resolve(context.Background(), t.TempDir(), id)
			assert.ErrorIs(t, err, descriptor.ErrUnknown)
		})
	}
}

func TestResolve_RespectsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := descriptor.Resolve(ctx, "", "claude")

	assert.ErrorIs(t, err, context.Canceled)
}

func TestResolve_ABrokenOverrideIsAnError(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "claude", "id: \"\"\n")

	_, err := descriptor.Resolve(context.Background(), home, "claude")

	assert.ErrorIs(t, err, descriptor.ErrInvalid,
		"asking for one provider by id must report why it is unusable")
}

func TestAll_EnumeratesTheEmbeddedSetSortedByID(t *testing.T) {
	list, err := descriptor.All(context.Background(), "")

	require.NoError(t, err)
	ids := idsOf(list)
	assert.Equal(t, []string{"claude", "codex"}, ids)
}

func TestAll_UnionsOnDiskIDsWithTheEmbeddedSet(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "zeta", minimalWithID("zeta", "zeta-cli"))

	list, err := descriptor.All(context.Background(), home)

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex", "zeta"}, idsOf(list))
}

func TestAll_ABrokenOverrideOmitsOneEntryNotTheList(t *testing.T) {
	home := t.TempDir()
	writeOverride(t, home, "broken", "id: \"\"\n")

	list, err := descriptor.All(context.Background(), home)

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, idsOf(list))
}

func TestAll_IgnoresNonYAMLAndDirectories(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "notadescriptor.yaml"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0o600))

	list, err := descriptor.All(context.Background(), home)

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, idsOf(list))
}

func TestAll_RespectsACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := descriptor.All(ctx, "")

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
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	require.Len(t, d.TerminalNotices, 1)
	assert.Equal(t, spec.TerminalNoticeUsageLimit, d.TerminalNotices[0].Kind)
	assert.Equal(t, "You've hit your usage limit", d.TerminalNotices[0].Needle)
	assert.True(t, d.TerminalNotices[0].EndsTurn)
}

func TestResolve_ShippedCodexDeclaresBothBlockingModals(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	needles := make([]string, 0, len(d.TerminalPrompts))
	for _, p := range d.TerminalPrompts {
		needles = append(needles, p.Needle)
	}
	assert.Contains(t, needles, "Press enter to continue")
	assert.Contains(t, needles, "Press enter to confirm or esc to go back")
}

func TestResolve_ShippedClaudeDeclaresNoNotices(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude")
	require.NoError(t, err)

	assert.Empty(t, d.TerminalNotices)
}

func TestShippedClaudeDeclaresHotswapTrue(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude")
	require.NoError(t, err)
	assert.True(t, d.Runtime.Hotswap, "claude keeps the PTY attached for the whole "+
		"session with hooks reporting alongside (design spec §3.5)")
}

// codex declares attach WITHOUT hotswap — confirmed live against codex-cli
// 0.149.1 that the app-server's LIVE-attach mechanism (a second client on the
// SAME connection, `codex resume {id} --remote`) does not work (fails
// outright — "no rollout found for thread id" — before any turn has
// completed) and is the wrong shape anyway (codex enforces one writer per
// thread). What DOES work, confirmed live: a bare, ordinary `codex resume
// {id}` — no --remote — once idle. So attach is idle-only sequential handoff
// (SwitchToTerminal in runner/attach.go), never a live/concurrent view, which
// is exactly what hotswap:false means. See codex.yaml's own comment.
func TestCodexDescriptor_DeclaresAttachWithoutHotswap(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	assert.NotEmpty(t, d.Runtime.API.Attach, "idle-only handoff needs a bare resume argv to fork")
	assert.False(t, d.Runtime.Hotswap, "codex hands its live turn over, it never shares it")
}

// TestShippedClaudeDeclaresBothSurfacesStartHere: claude's terminal PTY IS
// the CLI from the instant it spawns (hotswap: true, both faces live at
// once), so a brand-new chat may land directly on either surface.
func TestShippedClaudeDeclaresBothSurfacesStartHere(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "claude")
	require.NoError(t, err)

	require.Contains(t, d.Surfaces, "chat")
	require.Contains(t, d.Surfaces, "terminal")
	assert.True(t, d.Surfaces["chat"].StartHere)
	assert.True(t, d.Surfaces["terminal"].StartHere)
	assert.True(t, d.SurfaceStartHere("terminal"))
}

// TestCodexDescriptor_BothSurfacesAreStartHere: codex's terminal surface is
// fed by the HOOKS channel — the ordinary `codex` PTY every spawn already
// forks, live from the fork and naming no session — so a brand-new chat may
// land on it directly. The idle-only restriction belongs to `attach`
// (SwitchToTerminal, runner/attach.go), which is the api channel's way back
// onto an EXISTING session and is not involved in a birth.
func TestCodexDescriptor_BothSurfacesAreStartHere(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
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
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	assert.Equal(t, "api", d.Runtime.Transport)
	assert.NotEmpty(t, d.Runtime.API.Serve)
	assert.NotEmpty(t, d.Runtime.Hooks.Format, "hooks stay declared — see codex.yaml's own comment on why")

	// subagent_pre/subagent_post remain the known gap (B1's nested-thread/
	// collab-tool-call model does not fit StartSubagent/StopSubagent yet —
	// see codex.yaml's own comment). session_end stays on hooks too: its
	// dispatch is already a no-op on either transport, and no live-reachable
	// api equivalent was found — see codex.yaml's comment there.
	hooksOnly := []string{"subagent_pre", "subagent_post", "session_end"}
	for _, name := range hooksOnly {
		assert.Equal(t, "hooks", d.TransportFor(name),
			"event %q must stay on hooks — the API does not carry it", name)
	}
	// compact_pre/compact_post moved off hooks: confirmed live that
	// thread/compact/start's contextCompaction item rides the same
	// item/started/item/completed stream tool_pre/tool_post already consume
	// on the api transport — see codex.yaml's own comment.
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

// REGRESSION. full-auto used to keep codex's workspace-write sandbox active
// and only silence the approval prompt (--sandbox workspace-write
// --ask-for-approval never), never reaching codex's real unrestricted tier —
// the user's own complaint was that full-auto permissions "are not mapping
// to the real full-access codex permissions". Confirmed live against a real
// codex 0.154.0 binary: --ask-for-approval cannot be combined with
// --dangerously-bypass-approvals-and-sandbox at all (clap rejects the argv
// outright), so full-auto's apply list must REPLACE both prior pass_args
// with this one flag, never add it alongside them.
func TestCodexDescriptor_FullAutoLevelUsesTheRealBypassFlag(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
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

	// The api-transport equivalent (thread/start's sandbox/approvalPolicy
	// params) — confirmed via `codex app-server generate-json-schema`:
	// ThreadStartParams.sandbox is codex's own SandboxMode enum, which
	// includes "danger-full-access", and setting sandbox=danger-full-access +
	// approvalPolicy=never is exactly what --dangerously-bypass-approvals-
	// and-sandbox resolves to internally (confirmed live: both produce an
	// identical effective "approval: never / sandbox: danger-full-access").
	assert.Equal(t, map[string]string{
		"sandbox":        "danger-full-access",
		"approvalPolicy": "never",
	}, level.Vars)
}
