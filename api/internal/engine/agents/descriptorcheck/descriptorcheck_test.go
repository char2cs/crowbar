package descriptorcheck_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// The shipped descriptors are the reference: every rule passes on them.
func TestValidateAll_TheShippedDescriptorsAreClean(t *testing.T) {
	reports, err := descriptorcheck.ValidateAll("")
	require.NoError(t, err)
	ids := make([]string, 0, len(reports))
	for _, rep := range reports {
		ids = append(ids, rep.ID)
		assert.Empty(t, rep.Findings, "%s", rep.ID)
		assert.True(t, rep.OK(), rep.ID)
	}
	assert.Equal(t, []string{"claude", "codex"}, ids)
}

func TestValidateAll_AnOverrideShadowsTheShippedDescriptor(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
	override := filepath.Join(home, "descriptors", "codex.yaml")
	require.NoError(t, os.WriteFile(override, []byte("id: codex\n"), 0o600))

	reports, err := descriptorcheck.ValidateAll(home)
	require.NoError(t, err)
	for _, rep := range reports {
		if rep.ID == "codex" {
			assert.Equal(t, override, rep.Source)
			assert.False(t, rep.OK(), "an override missing its spawn block is blocked")
		}
	}
}

func errorRules(rep descriptorcheck.Report) []string {
	var out []string
	for _, f := range rep.Findings {
		if f.Severity == descriptorcheck.SeverityError {
			out = append(out, f.Rule)
		}
	}
	return out
}

// shippedAs is the shipped claude descriptor as a user's file: renamed so a test
// can tell which document runs, under id, then edited.
func shippedAs(t *testing.T, id string, edit func(string) string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "protocol", "internal", "descriptor", "descriptors-v3", "claude.yaml"))
	require.NoError(t, err)
	doc := strings.Replace(string(raw), "display_name: ", "display_name: My ", 1)
	doc = strings.Replace(doc, "\nid: claude\n", "\nid: "+id+"\n", 1)
	edited := edit(doc)
	require.NotEqual(t, doc, edited, "the edit applies")
	return []byte(edited)
}

func writeDescriptor(t *testing.T, home, id string, raw []byte) string {
	t.Helper()
	path := filepath.Join(home, "descriptors", id+".yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

// The schema is pre-release and nothing is carried over: an override written for
// another schema is refused, says so, and the shipped descriptor runs. The file
// stays exactly as the user wrote it.
func TestValidateAll_AnOverrideForAnotherSchemaFallsBackUntouched(t *testing.T) {
	home := t.TempDir()
	doc := shippedAs(t, "claude", func(s string) string {
		return strings.Replace(s, "\nid: claude\n", "\nid: claude\nschema_version: 3\n", 1)
	})
	override := writeDescriptor(t, home, "claude", doc)

	rep := reportFor(t, home, "claude")
	assert.True(t, rep.FellBack)
	f := findingFor(t, rep, "schema.version")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
	assert.Positive(t, f.Line)
	assert.Contains(t, f.Message, "written for schema 3")
	assert.Contains(t, f.Hint, "pre-release")
	require.NoError(t, descriptorcheck.NewGate().Require(home, "claude"), "the shipped descriptor runs")
	a, err := agents.New(agents.WithOverrideCheck(descriptorcheck.AcceptOverride)).Get(context.Background(), home, "claude")
	require.NoError(t, err)
	assert.NotContains(t, a.Display().Name, "My ")
	got, err := os.ReadFile(override)
	require.NoError(t, err)
	assert.Equal(t, doc, got, "the user's file is never rewritten")
}

// A custom descriptor in an older shape is blocked, never dropped: it is
// reported at the retired key with what to do, and the gate refuses it.
func TestValidateAll_ACustomDescriptorOfAnOlderShapeIsBlockedWithItsFix(t *testing.T) {
	home := t.TempDir()
	doc := shippedAs(t, "mine", func(s string) string {
		return strings.Replace(s, "\nsession:\n", "\nsession:\n  locate: { root: \"~/x\" }\n", 1)
	})
	writeDescriptor(t, home, "mine", doc)

	rep := reportFor(t, home, "mine")
	assert.False(t, rep.FellBack, "there is no shipped descriptor to run instead")
	assert.Equal(t, []string{"yaml.unknown_field"}, errorRules(rep))
	f := findingFor(t, rep, "yaml.unknown_field")
	assert.Equal(t, "session.locate", f.Path)
	assert.Positive(t, f.Line)
	assert.NotEmpty(t, f.Hint)
	require.ErrorIs(t, descriptorcheck.NewGate().Require(home, "mine"), descriptorcheck.ErrBlocked)
}

func TestValidate_ADescriptorFromAnotherSchemaNamesBoth(t *testing.T) {
	rep := descriptorcheck.Validate([]byte("id: x\nschema_version: 9\n"))
	f := findingFor(t, rep, "schema.version")
	assert.Equal(t, "x", rep.ID)
	assert.Equal(t, 2, f.Line)
	assert.Contains(t, f.Message, "written for schema 9 (line 2), this build reads 0")
}

func reportFor(t *testing.T, home, id string) descriptorcheck.Report {
	t.Helper()
	reports, err := descriptorcheck.ValidateAll(home)
	require.NoError(t, err)
	for _, r := range reports {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no report for %s in %+v", id, reports)
	return descriptorcheck.Report{}
}

// The real migration case: an override in today's vocabulary whose hooks still
// run the retired relay is refused once, by the finding that names the replacement.
func TestValidate_AShippedDescriptorStillWiredToTheRelayIsRefused(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "internal", "protocol", "internal", "descriptor", "descriptors-v3", "claude.yaml"))
	require.NoError(t, err)
	relayed := strings.Replace(string(raw), "{hook_curl} {hook_url}/telemetry", "{crowbar_hook} hook telemetry --segment {segid}", 1)
	require.NotEqual(t, string(raw), relayed, "the shipped descriptor posts its telemetry through curl")

	rep := descriptorcheck.Validate([]byte(relayed))
	f := findingFor(t, rep, "hooks.relay_retired")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
	assert.True(t, strings.HasPrefix(f.Path, "hooks_injection["), f.Path)
	assert.Equal(t, "post hooks to {hook_url}/<event> (HTTP) or through {hook_curl}", f.Hint)
	for _, other := range rep.Findings {
		assert.NotEqual(t, "template.unknown_var", other.Rule, "one cause, one finding")
	}
	assert.False(t, descriptorcheck.AcceptOverride([]byte(relayed)))
}

// An override with no shipped default to fall back to is still blocked.
func TestGate_AnOverrideWithNoShippedDefaultIsStillBlocked(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "acme.yaml"), []byte("id: acme\n"), 0o600))

	require.ErrorIs(t, descriptorcheck.NewGate().Require(home, "acme"), descriptorcheck.ErrBlocked)
	reports, err := descriptorcheck.ValidateAll(home)
	require.NoError(t, err)
	for _, r := range reports {
		assert.False(t, r.FellBack, r.ID)
	}
}

func findingFor(t *testing.T, rep descriptorcheck.Report, rule string) descriptorcheck.Finding {
	t.Helper()
	for _, f := range rep.Findings {
		if f.Rule == rule {
			return f
		}
	}
	t.Fatalf("no %s finding in %+v", rule, rep.Findings)
	return descriptorcheck.Finding{}
}

func TestValidate_ASyntaxErrorIsOneFindingWithItsLine(t *testing.T) {
	rep := descriptorcheck.Validate([]byte("id: x\nspawn: [\n"))
	f := findingFor(t, rep, "yaml.syntax")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
	assert.Positive(t, f.Line)
	assert.False(t, rep.OK())
}

// A typo in a key the lenient load ignores is exactly the silent failure the
// validator exists to catch.
func TestValidate_AnUnknownKeyIsAnErrorAtItsPath(t *testing.T) {
	rep := descriptorcheck.Validate(minimal("hooks_injecton:\n  - pass_arg: { arg: x }\n"))
	f := findingFor(t, rep, "yaml.unknown_field")
	assert.Equal(t, "hooks_injecton", f.Path)
	assert.Equal(t, lineOf(t, minimal("hooks_injecton:\n  - pass_arg: { arg: x }\n"), "hooks_injecton:"), f.Line)
}

// One channel per process: hook wiring in config_injection reaches the
// app-server too, and every event would arrive twice.
func TestValidate_FindingsAreOrderedByLine(t *testing.T) {
	rep := descriptorcheck.Validate(minimal(
		"session:\n  resume: { arg: \"--resume\" }\nmcp_injection:\n  - pass_arg: { arg: \"{nope}\" }\n"))
	require.GreaterOrEqual(t, len(rep.Findings), 2)
	for i := 1; i < len(rep.Findings); i++ {
		assert.LessOrEqual(t, rep.Findings[i-1].Line, rep.Findings[i].Line)
	}
}

// minimal is the smallest descriptor every load rule accepts, plus extra.
func minimal(extra string) []byte {
	return []byte("id: mini\nspawn:\n  cmd: mini\n  interactive_required: true\nruntime:\n  transport: hooks\n  hooks: { format: json, delivery: http }\n" + extra)
}

// complete is minimal plus a wired lifecycle, so only extra can fail it.
func complete(extra string) []byte {
	return minimal(completeLifecycle + extra)
}

const completeLifecycle = `hooks_injection:
  - pass_arg: { arg: --hook, value: "{hook_curl} {hook_url}" }
events:
  session_start: { required: [session_id], in: SessionStart, map: { session_id: session_id } }
  user_prompt: { required: [session_id], in: UserPromptSubmit, map: { session_id: session_id, message: prompt } }
  turn_stop: { required: [session_id], in: Stop, map: { session_id: session_id, message: last_assistant_message } }
`

func lineOf(t *testing.T, raw []byte, prefix string) int {
	t.Helper()
	for i, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, prefix) {
			return i + 1
		}
	}
	t.Fatalf("no line starts with %q", prefix)
	return 0
}
