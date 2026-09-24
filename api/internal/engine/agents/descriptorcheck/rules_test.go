package descriptorcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

func TestValidate_AResumeWithoutLocateIsAWarning(t *testing.T) {
	rep := descriptorcheck.Validate(minimal("session:\n  resume: { arg: \"--resume {id}\" }\n"))
	f := findingFor(t, rep, "session.resume_unverified")
	assert.Equal(t, descriptorcheck.SeverityWarning, f.Severity)
	assert.True(t, rep.OK(), "a warning does not block the descriptor")
}

func TestValidate_TheSessionRulesRefuseAnUnusableLadder(t *testing.T) {
	cases := map[string]struct {
		yaml, rule, path string
	}{
		"resume without id": {"session:\n  resume: { arg: \"--resume\" }\n", "session.resume_id", "session.resume.arg"},
		"no root":           {locate("", "", `["{id}.jsonl"]`), "session.locate_root", "session.locate"},
		"relative root":     {locate("sessions", "", `["{id}.jsonl"]`), "session.locate_root", "session.locate.root"},
		"no glob":           {locate("~/.x", "", `[]`), "session.locate_glob", "session.locate"},
		"glob without id":   {locate("~/.x", "X_HOME", `["*.jsonl"]`), "session.locate_glob", "session.locate.glob[0]"},
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

func locate(root, env, glob string) string {
	s := "session:\n  resume: { arg: \"--resume {id}\" }\n  locate:\n"
	if root != "" {
		s += "    root: " + root + "\n"
	}
	if env != "" {
		s += "    root_env: " + env + "\n"
	}
	return s + "    glob: " + glob + "\n"
}
