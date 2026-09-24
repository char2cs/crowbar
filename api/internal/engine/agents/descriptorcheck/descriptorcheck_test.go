package descriptorcheck_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
