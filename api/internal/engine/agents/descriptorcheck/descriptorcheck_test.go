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

// A user who copied the pre-audit shipped descriptor into ~/.crowbar/descriptors
// has an override the current rules refuse (retired keys, hooks no longer
// wired the old way). It must not take the provider down: the shipped
// descriptor runs, and the status report says the override was refused and why.
func TestValidateAll_AnOverrideCopiedFromAnOldShippedDescriptorFallsBack(t *testing.T) {
	for _, id := range []string{"claude", "codex"} {
		t.Run(id, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, "descriptors")
			require.NoError(t, os.MkdirAll(dir, 0o750))
			old, err := os.ReadFile(filepath.Join("testdata", "base-70ec430", id+".yaml"))
			require.NoError(t, err)
			// Renamed so the test can tell which document actually runs.
			old = []byte(strings.Replace(string(old), "display_name: ", "display_name: My ", 1))
			override := filepath.Join(dir, id+".yaml")
			require.NoError(t, os.WriteFile(override, old, 0o600))

			reports, err := descriptorcheck.ValidateAll(home)
			require.NoError(t, err)
			var rep descriptorcheck.Report
			for _, r := range reports {
				if r.ID == id {
					rep = r
				}
			}
			assert.Equal(t, override, rep.Source)
			assert.True(t, rep.FellBack, "the shipped descriptor runs in its place")
			findingFor(t, rep, "hooks.in_config_injection")
			assert.False(t, descriptorcheck.AcceptOverride(old))

			require.NoError(t, descriptorcheck.NewGate().Require(home, id),
				"the provider stays enabled on the shipped descriptor")
			a, err := agents.New(agents.WithOverrideCheck(descriptorcheck.AcceptOverride)).
				Get(context.Background(), home, id)
			require.NoError(t, err)
			assert.NotContains(t, a.Display().Name, "My ", "the refused override is not what runs")
		})
	}
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
  - pass_arg: { arg: --hook, value: "{crowbar_hook} hook any --segment {segid}" }
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
