package descriptorcheck_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

func TestValidate_AShellAsTheCommandIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate([]byte(strings.Replace(string(complete("")), "cmd: mini", "cmd: /bin/sh", 1)))
	f := findingFor(t, rep, "argv.shell")
	assert.Equal(t, "spawn.cmd", f.Path)
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
}

func TestValidate_AShellWithAFixedScriptIsOnlyAWarning(t *testing.T) {
	rep := descriptorcheck.Validate([]byte(strings.Replace(string(minimal(
		"hooks_injection:\n  - set_env: { name: H, value: \"{crowbar_hook} hook any --segment {segid}\" }\n"+
			"events:\n  session_start: { in: s, map: { session_id: s } }\n"+
			"  user_prompt: { in: u, map: { session_id: s, message: p } }\n"+
			"  turn_stop: { in: t, map: { session_id: s, message: m } }\n")),
		"cmd: mini", "cmd: sh\n  args: [\"-c\", \"exec cat\"]", 1)))
	f := findingFor(t, rep, "argv.shell")
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
	assert.True(t, rep.OK(), "%+v", rep.Findings)
}

// A hook command runs through the CLI's shell, so a value the user or the
// provider controls must never be spliced into one.
func TestValidate_AUserValueInAHookCommandIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate(complete(
		"mcp_injection:\n  - pass_arg: { arg: -c, value: 'x=\"{crowbar_hook} hook turn_stop --cwd {cwd}\"' }\n"))
	f := findingFor(t, rep, "hooks.unsafe_placeholder")
	assert.Equal(t, "mcp_injection[0].pass_arg.value", f.Path)
	assert.Contains(t, f.Message, "{cwd}")
}

func TestValidate_TheHookCommandPlaceholdersAreAccepted(t *testing.T) {
	rep := descriptorcheck.Validate(complete(
		"mcp_injection:\n  - pass_arg: { arg: -c, value: 'x=\"{crowbar_hook} hook a --segment {segid} --provider {provider} {scope_flags}\" y={cwd_json}' }\n"))
	assert.True(t, rep.OK(), "%+v", rep.Findings)
}

func TestValidate_ShellSyntaxInAnArgvIsAWarning(t *testing.T) {
	rep := descriptorcheck.Validate([]byte(strings.Replace(string(complete("")),
		"interactive_required: true", "interactive_required: true\n  args: [\"--dir=$(pwd)\"]", 1)))
	f := findingFor(t, rep, "argv.shell_syntax")
	assert.Equal(t, "spawn.args[0]", f.Path)
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
}

func TestValidate_AnEnvNameThatIsNotOneIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate([]byte(strings.Replace(string(complete("")),
		"interactive_required: true", "interactive_required: true\n  env: { clear: [\"A=B; rm -rf\"] }", 1)))
	f := findingFor(t, rep, "env.name")
	assert.Equal(t, "spawn.env.clear[0]", f.Path)
}

func TestValidate_AVersionRangeNothingCanPassIsAnError(t *testing.T) {
	cases := map[string]struct{ yaml, path string }{
		"not numeric": {"protocol_version: { min: \"v1\" }\n", "protocol_version.min"},
		"inverted":    {"protocol_version: { min: \"0.160\", max: \"0.156.1\" }\n", "protocol_version"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := findingFor(t, descriptorcheck.Validate(complete(tc.yaml)), "version.range")
			assert.Equal(t, tc.path, f.Path)
		})
	}
	assert.True(t, descriptorcheck.Validate(complete("protocol_version: { min: \"0.150\", max: \"0.156.1\" }\n")).OK())
}
