package descriptorcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

func TestValidate_AResumeWithoutARecognisedRefusalIsAWarning(t *testing.T) {
	rep := descriptorcheck.Validate(complete("session:\n  resume: { arg: \"--resume {id}\" }\n  persists: turn_start\n"))
	f := findingFor(t, rep, "session.refusal_unrecognised")
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
	assert.True(t, rep.OK(), "a warning does not block the descriptor")
}

// Over an api connection a refused resume is an error reply on the turn it
// was asked for; unrecognised, the chat keeps a session the server lost.
func TestValidate_AnAPIProvidersResumeWithoutARecognisedRefusalIsAnError(t *testing.T) {
	api := "id: api\nspawn:\n  cmd: api\n  interactive_required: true\nruntime:\n  transport: api\n" +
		"  api: { protocol: jsonrpc2, serve: [api, serve] }\n"
	f := findingFor(t, descriptorcheck.Validate([]byte(api+"session:\n  resume: { arg: \"--resume {id}\" }\n")),
		"session.refusal_unrecognised")
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
}

func TestValidate_TheSessionRulesRefuseAnUnusableSession(t *testing.T) {
	cases := map[string]struct {
		yaml, rule, path string
	}{
		"resume without id":   {"session:\n  resume: { arg: \"--resume\" }\n", "session.resume_id", "session.resume.arg"},
		"assign without id":   {"session:\n  assign: { arg: \"--session-id\" }\n", "session.assign_id", "session.assign.arg"},
		"refused exit 0":      {refused("0", `["gone"]`), "session.refused_exit", "session.refused"},
		"no needles":          {refused("1", `[]`), "session.refused_needles", "session.refused"},
		"an empty needle":     {refused("1", `[""]`), "session.refused_needles", "session.refused"},
		"unknown persists":    {"session:\n  persists: turn_middle\n", "session.persists", "session.persists"},
		"resume, no persists": {"session:\n  resume: { arg: \"--resume {id}\" }\n", "session.persists_missing", "session.resume"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rep := descriptorcheck.Validate(minimal(tc.yaml))
			f := findingFor(t, rep, tc.rule)
			assert.Equal(t, tc.path, f.Path)
			assert.Equal(t, descriptorcheck.SeverityError, f.Severity)
			assert.Positive(t, f.Line)
		})
	}
}

func TestValidate_AnAPIProvidersRefusalNamesItsRPCCode(t *testing.T) {
	api := "id: api\nspawn:\n  cmd: api\n  interactive_required: true\nruntime:\n  transport: api\n" +
		"  api: { protocol: jsonrpc2, serve: [api, serve] }\n"
	f := findingFor(t, descriptorcheck.Validate([]byte(api+refused("1", `["gone"]`))), "session.refused_rpc")
	assert.Equal(t, "session.refused", f.Path)
	assert.Equal(t, descriptorcheck.SeverityError, f.Severity)

	withCode := "session:\n  resume: { arg: \"--resume {id}\" }\n  refused: { exit_code: 1, rpc_code: -32600, needles: [gone] }\n"
	for _, f := range descriptorcheck.Validate([]byte(api + withCode)).Findings {
		assert.NotEqual(t, "session.refused_rpc", f.Rule)
	}
}

func TestValidate_AnUnknownTemplateVariableIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate(minimal("mcp_injection:\n  - pass_arg: { arg: --dir, value: \"{cwd_jsno}\" }\n"))
	f := findingFor(t, rep, "template.unknown_var")
	assert.Equal(t, "mcp_injection[0].pass_arg.value", f.Path)
	assert.Contains(t, f.Message, "{cwd_jsno}")
	assert.Contains(t, f.Hint, "cwd_json")
}

func TestValidate_AnUnknownInjectVerbIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate(minimal("context_inject:\n  - pass_flag: { arg: x }\n"))
	f := findingFor(t, rep, "inject.unknown_verb")
	assert.Equal(t, "context_inject[0]", f.Path)
}

func TestValidate_HookWiringOutsideHooksInjectionIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate(minimal(
		"config_injection:\n  - pass_arg: { arg: -c, value: \"hooks.Stop={crowbar_hook} hook turn_stop\" }\n"))
	f := findingFor(t, rep, "hooks.in_config_injection")
	assert.Equal(t, "config_injection[0]", f.Path)
}

func TestValidate_CurlHookWiringOutsideHooksInjectionIsAnError(t *testing.T) {
	rep := descriptorcheck.Validate(minimal(
		"config_injection:\n  - pass_arg: { arg: -c, value: \"hooks.Stop={hook_curl} {hook_url}/turn_stop\" }\n"))
	f := findingFor(t, rep, "hooks.in_config_injection")
	assert.Equal(t, "config_injection[0]", f.Path)
}

func TestValidate_HookEventsWithNoWiringAreAnError(t *testing.T) {
	rep := descriptorcheck.Validate(minimal("events:\n  session_start:\n    in: SessionStart\n    map: { session_id: session_id }\n"))
	f := findingFor(t, rep, "hooks.not_wired")
	assert.Equal(t, "hooks_injection", f.Path)
}

func TestValidate_AnAPIProviderNeedsAServeArgv(t *testing.T) {
	rep := descriptorcheck.Validate([]byte(
		"id: api\nspawn:\n  cmd: api\n  interactive_required: true\nruntime:\n  transport: api\n  api: { protocol: jsonrpc2 }\n"))
	f := findingFor(t, rep, "runtime.api_serve")
	assert.Equal(t, "runtime.api", f.Path)
	assert.Equal(t, 7, f.Line)
}

func refused(code, needles string) string {
	return "session:\n  resume: { arg: \"--resume {id}\" }\n  refused: { exit_code: " + code + ", needles: " + needles + " }\n"
}
