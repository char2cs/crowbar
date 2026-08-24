package descriptor_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

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

func TestShippedDescriptors_DeclareHotswapTrue(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		d, err := descriptor.Resolve(context.Background(), t.TempDir(), id)
		require.NoError(t, err)
		assert.True(t, d.Runtime.Hotswap, "%s must declare hotswap — both keep the PTY "+
			"attached for the whole session with hooks reporting alongside (design spec §3.5)", id)
	}
}

func TestCodexDescriptor_IsMergedMixedTransport(t *testing.T) {
	d, err := descriptor.Resolve(context.Background(), t.TempDir(), "codex")
	require.NoError(t, err)

	assert.Equal(t, "api", d.Runtime.Transport)
	assert.NotEmpty(t, d.Runtime.API.Serve)
	assert.NotEmpty(t, d.Runtime.API.Attach)
	assert.NotEmpty(t, d.Runtime.Hooks.Format, "hooks stay declared — the attached TUI still fires them")

	hooksOnly := []string{"subagent_pre", "subagent_post", "compact_pre", "compact_post", "session_end"}
	for _, name := range hooksOnly {
		assert.Equal(t, "hooks", d.TransportFor(name),
			"event %q must stay on hooks — the API does not carry it", name)
	}
	apiOnly := []string{
		"session_start", "user_prompt", "turn_stop", "tool_pre", "tool_post",
		"message_delta", "permission", "elicitation", "telemetry", "interrupt", "compact_start",
	}
	for _, name := range apiOnly {
		assert.Equal(t, "api", d.TransportFor(name), "event %q must be on the api default", name)
	}
}

func TestExperimentalCodexAPIDescriptorIsGone(t *testing.T) {
	_, err := os.Stat("descriptors-v3/experimental/codex-api.yaml")
	assert.True(t, os.IsNotExist(err), "codex-api.yaml is merged into codex.yaml — it must not exist alongside it")
}
