package descriptorcheck

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func (d document) finding(rule string, sev Severity, path, msg, hint string) Finding {
	return Finding{Rule: rule, Severity: sev, Path: path, Line: d.line(path), Message: msg, Hint: hint}
}

// sessionRules check what a chat's session needs: resume and assign argv that
// name the session, and a refusal Crowbar can recognise.
func sessionRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	s := d.Session
	if s.Resume != nil && !strings.Contains(s.Resume.Arg, "{id}") {
		out = append(out, doc.finding("session.resume_id", SeverityError, "session.resume.arg",
			"session.resume.arg never names the session ({id})",
			`write the resume argv with {id} where the CLI takes the session, e.g. "<flag> {id}"`))
	}
	if s.Assign != nil && !strings.Contains(s.Assign.Arg, "{id}") {
		out = append(out, doc.finding("session.assign_id", SeverityError, "session.assign.arg",
			"session.assign.arg never names the session ({id})",
			`write the assign argv with {id} where the CLI takes the session, e.g. "<flag> {id}"`))
	}
	if s.Resume != nil && s.Refused == nil {
		// Over an api connection the refusal is an error reply on the asked-for turn.
		sev := SeverityWarning
		if d.Runtime.Transport == string(spec.ChannelAPI) {
			sev = SeverityError
		}
		out = append(out, doc.finding("session.refusal_unrecognised", sev, "session.resume",
			"a refused resume is never recognised: a chat keeps a session the CLI cannot load",
			"declare session.refused (exit_code, needles) with the line the CLI prints when it cannot resume"))
	}
	if s.Resume != nil && s.Persists == "" {
		out = append(out, doc.finding("session.persists_missing", SeverityError, "session.resume",
			"a resumable session never says when the CLI has written it: a chat may resume one that is not there",
			`set session.persists to "turn_start" or "turn_end"`))
	}
	if s.Persists != "" && s.Persists != "turn_start" && s.Persists != "turn_end" {
		out = append(out, doc.finding("session.persists", SeverityError, "session.persists",
			"session.persists is "+s.Persists, `write "turn_start" or "turn_end": when the CLI has written a session it can resume`))
	}
	return append(out, refusalRules(doc, d)...)
}

// refusalRules check that a refused resume is recognised on the channel the
// provider speaks.
func refusalRules(doc document, d *spec.Descriptor) []Finding {
	s := d.Session
	if s.Refused == nil {
		return nil
	}
	var out []Finding
	if d.Runtime.Transport == string(spec.ChannelAPI) && s.Refused.RPCCode == 0 {
		out = append(out, doc.finding("session.refused_rpc", SeverityError, "session.refused",
			"an api provider's refused resume names no rpc_code: a refusal over the connection is never recognised",
			"set session.refused.rpc_code to the JSON-RPC error code the server answers a lost session with"))
	}
	if s.Refused.ExitCode == 0 {
		out = append(out, doc.finding("session.refused_exit", SeverityError, "session.refused",
			"session.refused.exit_code is 0, the code of a CLI that succeeded", "set the code the CLI exits with when it refuses"))
	}
	if len(s.Refused.Needles) == 0 || slices.Contains(s.Refused.Needles, "") {
		out = append(out, doc.finding("session.refused_needles", SeverityError, "session.refused",
			"session.refused needs non-empty needles", "add the text of the line the CLI prints when it refuses a resume"))
	}
	return out
}

var templateVarRE = regexp.MustCompile(`\{([a-z_][a-z0-9_.]*)\}`)

// argvField is one templated string in a process's argv, with its YAML path.
type argvField struct {
	path  string
	value string
}

// templateRules refuse an argv template naming a variable Crowbar never
// fills, which would reach the CLI as a literal "{typo}".
func templateRules(doc document, d *spec.Descriptor) []Finding {
	known := models.TemplateVars()
	var out []Finding
	for _, f := range argvFields(d) {
		for _, m := range templateVarRE.FindAllStringSubmatch(f.value, -1) {
			name := m[1]
			if slices.Contains(known, name) || strings.HasPrefix(name, "permission.") {
				continue
			}
			out = append(out, doc.finding("template.unknown_var", SeverityError, f.path,
				fmt.Sprintf("{%s} is not a template variable Crowbar fills", name),
				"use one of: "+strings.Join(sortedCopy(known), ", ")))
		}
	}
	out = append(out, verbRules(doc, d)...)
	return out
}

func argvFields(d *spec.Descriptor) []argvField {
	var out []argvField
	list := func(prefix string, argv []string) {
		for i, a := range argv {
			out = append(out, argvField{fmt.Sprintf("%s[%d]", prefix, i), a})
		}
	}
	list("spawn.args", d.Spawn.Args)
	list("runtime.api.serve", d.Runtime.API.Serve)
	list("runtime.api.attach", d.Runtime.API.Attach)
	if d.Session.Resume != nil {
		out = append(out, argvField{"session.resume.arg", d.Session.Resume.Arg})
	}
	if d.Session.Assign != nil {
		out = append(out, argvField{"session.assign.arg", d.Session.Assign.Arg})
	}
	for name, steps := range injections(d) {
		for i, step := range steps {
			for _, key := range sortedKeys(step.Args) {
				if v, ok := step.Args[key].(string); ok {
					out = append(out, argvField{fmt.Sprintf("%s[%d].%s.%s", name, i, step.Verb, key), v})
				}
			}
		}
	}
	return out
}

func injections(d *spec.Descriptor) map[string][]spec.InjectStep {
	return map[string][]spec.InjectStep{
		"config_injection":      d.ConfigInjection,
		"hooks_injection":       d.HooksInjection,
		"mcp_injection":         d.MCPInject,
		"context_inject":        d.ContextInject,
		"resume_context_inject": d.ResumeContextInject,
	}
}

func verbRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	for _, name := range sortedKeys(injections(d)) {
		for i, step := range injections(d)[name] {
			if spawn.KnownVerb(step.Verb) {
				continue
			}
			out = append(out, doc.finding("inject.unknown_verb", SeverityError, fmt.Sprintf("%s[%d]", name, i),
				fmt.Sprintf("inject verb %q does not exist", step.Verb), "use pass_arg, set_env or write_file"))
		}
	}
	return out
}

// channelRules keep one channel per process: hook wiring belongs to the PTY
// (hooks_injection), and an api-transport provider needs a serve argv.
func channelRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	if usesHooks(d) && len(d.HooksInjection) == 0 && !wiresHooks(d.ConfigInjection) {
		out = append(out, doc.finding("hooks.not_wired", SeverityError, "hooks_injection",
			"events arrive over hooks but no process is told where to send them",
			"declare hooks_injection steps that point the CLI's hooks at {crowbar_hook} or {hook_url}"))
	}
	for i, step := range d.ConfigInjection {
		if wiresHooks([]spec.InjectStep{step}) {
			out = append(out, doc.finding("hooks.in_config_injection", SeverityError, fmt.Sprintf("config_injection[%d]", i),
				"hook wiring in config_injection also reaches the app-server, so events arrive twice",
				"move hook wiring to hooks_injection, which is applied to a PTY only"))
		}
	}
	if d.Runtime.Transport == string(spec.ChannelAPI) && len(d.Runtime.API.Serve) == 0 {
		out = append(out, doc.finding("runtime.api_serve", SeverityError, "runtime.api",
			"runtime.transport is api but runtime.api.serve is empty", "declare the argv that starts the provider's app-server"))
	}
	return out
}

func usesHooks(d *spec.Descriptor) bool {
	for name := range d.Events {
		if d.TransportFor(name) == string(spec.ChannelHooks) || d.Events[name].Hooks != nil {
			return true
		}
	}
	for _, s := range d.Surfaces {
		if s.Channel == spec.ChannelHooks {
			return true
		}
	}
	return false
}

func wiresHooks(steps []spec.InjectStep) bool {
	for _, step := range steps {
		for _, v := range step.Args {
			if s, ok := v.(string); ok && firstHookCommand(s) >= 0 {
				return true
			}
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
