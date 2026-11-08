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

// historyDir holds every distinct descriptor document develop ever shipped, as
// it shipped: users copied them as overrides and as the start of their own.
const historyDir = "testdata/history"

// unmigratable names, per historic document, the errors no upgrade can fix
// because the fact was never in the file: an api model choice carried only on
// argv (refused since 2026-09-23) needs the provider's own api call.
var unmigratable = map[string][]string{
	"2026-09-03-5fee7690-codex.yaml": {"load.selection_api_carrier"},
	"2026-09-08-9b98c585-codex.yaml": {"load.selection_api_carrier"},
	"2026-09-09-1362209c-codex.yaml": {"load.selection_api_carrier"},
	"2026-09-10-25dda19c-codex.yaml": {"load.selection_api_carrier"},
	"2026-09-12-d9c2b3d5-codex.yaml": {"load.selection_api_carrier"},
	"2026-09-15-49399113-codex.yaml": {"load.selection_api_carrier"},
	"2026-09-21-6287d739-codex.yaml": {"load.selection_api_carrier"},
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

// Every shape a descriptor ever had keeps working: as a provider of its own and
// as an override of a shipped one, it loads, runs as written and says what the
// upgrade changed. A schema change that breaks one without a transition fails here.
func TestValidateAll_EveryHistoricDescriptorShapeStillRuns(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(historyDir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		name := filepath.Base(f)
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(f)
			require.NoError(t, err)
			id := descriptorcheck.Validate(raw).ID
			// Renamed so the test can tell which document actually runs.
			mine := []byte(strings.Replace(string(raw), "display_name: ", "display_name: My ", 1))
			custom := []byte(strings.Replace(string(mine), "id: "+id+"\n", "id: mine\n", 1))
			require.NotEqual(t, string(mine), string(custom), "the id line is renamed")

			for as, doc := range map[string][]byte{id: mine, "mine": custom} {
				home := t.TempDir()
				override := filepath.Join(home, "descriptors", as+".yaml")
				require.NoError(t, os.MkdirAll(filepath.Dir(override), 0o750))
				require.NoError(t, os.WriteFile(override, doc, 0o600))

				rep := reportFor(t, home, as)
				assert.Equal(t, override, rep.Source)
				assert.Equal(t, unmigratable[name], errorRules(rep), "%s as %s", name, as)
				assert.NotEmpty(t, schemaFindings(rep), "the upgrade says what it changed")
				if len(unmigratable[name]) > 0 {
					continue
				}
				assert.False(t, rep.FellBack, "the user's document runs")
				require.NoError(t, descriptorcheck.NewGate().Require(home, as))
				a, err := agents.New(agents.WithOverrideCheck(descriptorcheck.AcceptOverride)).
					Get(context.Background(), home, as)
				require.NoError(t, err)
				assert.Contains(t, a.Display().Name, "My ", "the user's document is the one that runs")

				up, err := descriptorcheck.Upgrade(doc)
				require.NoError(t, err)
				again, err := descriptorcheck.Upgrade(up.Raw)
				require.NoError(t, err)
				assert.Empty(t, again.Changes, "an upgraded document needs no second upgrade")
				assert.Empty(t, schemaFindings(descriptorcheck.Validate(up.Raw)), "nothing is left to report once rewritten")
			}
		})
	}
}

// What no upgrade can fix stays an error the user can act on: the file, the key
// and line, and what to write. An override of a shipped provider falls back to it.
func TestValidateAll_AnUnmigratableOldOverrideIsReportedAndTheShippedOneRuns(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(historyDir, "2026-09-21-6287d739-codex.yaml"))
	require.NoError(t, err)
	home := t.TempDir()
	override := filepath.Join(home, "descriptors", "codex.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(override), 0o750))
	require.NoError(t, os.WriteFile(override, raw, 0o600))

	rep := reportFor(t, home, "codex")
	assert.True(t, rep.FellBack)
	f := findingFor(t, rep, "load.selection_api_carrier")
	assert.Equal(t, "model", f.Path)
	assert.Positive(t, f.Line)
	assert.Contains(t, f.Hint, "api_apply")
	require.NoError(t, descriptorcheck.NewGate().Require(home, "codex"), "the shipped descriptor runs")
	assert.FileExists(t, override, "the user's file is never moved")
}

// A requirement newer than the document is a warning: the provider runs without
// what it enables. The same gap in a document of today's schema blocks.
func TestValidate_ARequirementNewerThanTheDocumentIsAWarning(t *testing.T) {
	notice := "terminal_notices:\n  - kind: usage_limit\n    needle: \"limit reached\"\n"
	old := []byte(strings.Replace(string(minimal(notice)), "schema_version: 3\n", "", 1))
	f := findingFor(t, descriptorcheck.Validate(old), "terminal_notices.anchor_missing")
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
	assert.Contains(t, f.Message, "runs without it")

	f = findingFor(t, descriptorcheck.Validate(minimal(notice)), "terminal_notices.anchor_missing")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
}

func TestValidate_ADescriptorFromANewerSchemaNamesTheVersion(t *testing.T) {
	rep := descriptorcheck.Validate([]byte("id: x\nschema_version: 9\n"))
	f := findingFor(t, rep, "schema.version")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
	assert.Equal(t, "x", rep.ID)
	assert.Contains(t, f.Hint, "up to 3")
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

func schemaFindings(rep descriptorcheck.Report) []descriptorcheck.Finding {
	var out []descriptorcheck.Finding
	for _, f := range rep.Findings {
		if strings.HasPrefix(f.Rule, "schema.") {
			out = append(out, f)
		}
	}
	return out
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
	return []byte("schema_version: 3\nid: mini\nspawn:\n  cmd: mini\n  interactive_required: true\nruntime:\n  transport: hooks\n  hooks: { format: json, delivery: http }\n" + extra)
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
