package descriptorcheck

import (
	"fmt"
	"path/filepath"
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

// sessionRules check what the resume ladder needs: a resume argv that names
// the session, and a way to find the session on disk before trusting it.
func sessionRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	s := d.Session
	if s.Resume != nil && !strings.Contains(s.Resume.Arg, "{id}") {
		out = append(out, doc.finding("session.resume_id", SeverityError, "session.resume.arg",
			"session.resume.arg never names the session ({id})",
			`write the resume argv with {id}, e.g. "--resume {id}"`))
	}
	if s.Resume != nil && s.Locate == nil {
		out = append(out, doc.finding("session.resume_unverified", SeverityWarning, "session.resume",
			"resume ids are never verified: a lost session is found only when the CLI exits",
			"declare session.locate (root, root_env, glob with {id}) so a missing session continues from the transcript at once"))
	}
	if s.Locate == nil {
		return out
	}
	if s.Locate.Root == "" && s.Locate.RootEnv == "" {
		out = append(out, doc.finding("session.locate_root", SeverityError, "session.locate",
			"session.locate declares neither root nor root_env", `set root (e.g. "~/.codex") and, if the CLI honours one, root_env`))
	}
	if r := s.Locate.Root; r != "" && !filepath.IsAbs(r) && !strings.HasPrefix(r, "~/") {
		out = append(out, doc.finding("session.locate_root", SeverityError, "session.locate.root",
			fmt.Sprintf("session.locate.root %q is relative", r), `use an absolute path or one under "~/"`))
	}
	if len(s.Locate.Glob) == 0 {
		out = append(out, doc.finding("session.locate_glob", SeverityError, "session.locate",
			"session.locate declares no glob", `add a glob naming the session file, e.g. "projects/*/{id}.jsonl"`))
	}
	for i, g := range s.Locate.Glob {
		if strings.Count(g, "{id}") != 1 {
			out = append(out, doc.finding("session.locate_glob", SeverityError, fmt.Sprintf("session.locate.glob[%d]", i),
				fmt.Sprintf("glob %q must name the session id exactly once ({id})", g), "put {id} where the session id appears in the file name"))
		}
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
			"declare hooks_injection steps that point the CLI's hooks at {crowbar_hook}"))
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
			if s, ok := v.(string); ok && strings.Contains(s, "{crowbar_hook} hook ") {
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
