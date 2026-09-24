package descriptorcheck

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// hookCommandVars are the only placeholders a hook command may carry. The
// vendor CLI runs a hook command through its shell, so anything a user or
// provider controls ({message}, {cwd}, {context}, …) would be interpreted there.
var hookCommandVars = []string{"crowbar_hook", "segid", "provider", "scope_flags"}

var shells = []string{"sh", "bash", "zsh", "dash", "ksh", "fish", "cmd", "cmd.exe", "powershell", "pwsh"}

var (
	envNameRE     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	versionRE     = regexp.MustCompile(`^\d+(\.\d+)*$`)
	shellSyntaxRE = regexp.MustCompile("\\$\\(|`|\\$\\{")
)

// safetyRules refuse what would let a descriptor — possibly one a user
// uploaded — run something other than its CLI: a shell as the command, shell
// syntax in an argv, a user-controlled value inside a hook command, an env
// name that is not one, or a version range nothing can compare against.
func safetyRules(doc document, d *spec.Descriptor) []Finding {
	var out []Finding
	out = append(out, shellCommandRules(doc, d)...)
	for _, f := range argvFields(d) {
		out = append(out, hookPlaceholderRules(doc, f)...)
		if !strings.Contains(f.value, "{crowbar_hook} hook ") && shellSyntaxRE.MatchString(f.value) {
			out = append(out, doc.finding("argv.shell_syntax", SeverityWarning, f.path,
				"shell syntax in an argv is passed to the CLI literally, never expanded",
				"use a template variable instead of shell expansion"))
		}
	}
	for i, name := range d.Spawn.Env.Clear {
		if !envNameRE.MatchString(name) {
			out = append(out, doc.finding("env.name", SeverityError, fmt.Sprintf("spawn.env.clear[%d]", i),
				fmt.Sprintf("%q is not an environment variable name", name), "use letters, digits and underscores"))
		}
	}
	return append(out, versionRules(doc, d)...)
}

func shellCommandRules(doc document, d *spec.Descriptor) []Finding {
	commands := map[string]string{"spawn.cmd": d.Spawn.Cmd}
	if len(d.Runtime.API.Serve) > 0 {
		commands["runtime.api.serve[0]"] = d.Runtime.API.Serve[0]
	}
	if len(d.Runtime.API.Attach) > 0 {
		commands["runtime.api.attach[0]"] = d.Runtime.API.Attach[0]
	}
	// A shell is only an injection when a value Crowbar fills reaches its argv;
	// a fixed script (a test stub) is merely unusual.
	sev, why := SeverityWarning, "a shell as the CLI runs whatever its script says"
	if interpolatesArgv(d) {
		sev, why = SeverityError, "a shell as the CLI would interpret the values Crowbar passes it"
	}
	var out []Finding
	for _, path := range sortedKeys(commands) {
		if slices.Contains(shells, filepath.Base(commands[path])) {
			out = append(out, doc.finding("argv.shell", sev, path,
				fmt.Sprintf("%q is a shell: %s", commands[path], why), "name the provider's own CLI binary"))
		}
	}
	return out
}

// interpolatesArgv reports whether any argv element carries a template
// variable: the spawn args, or a pass_arg of any injection.
func interpolatesArgv(d *spec.Descriptor) bool {
	for _, a := range d.Spawn.Args {
		if templateVarRE.MatchString(a) {
			return true
		}
	}
	for _, steps := range injections(d) {
		for _, step := range steps {
			if step.Verb != "pass_arg" {
				continue
			}
			for _, v := range step.Args {
				if s, ok := v.(string); ok && templateVarRE.MatchString(s) {
					return true
				}
			}
		}
	}
	return false
}

func hookPlaceholderRules(doc document, f argvField) []Finding {
	var out []Finding
	for _, command := range hookCommands(f.value) {
		for _, m := range templateVarRE.FindAllStringSubmatch(command, -1) {
			if slices.Contains(hookCommandVars, m[1]) {
				continue
			}
			out = append(out, doc.finding("hooks.unsafe_placeholder", SeverityError, f.path,
				fmt.Sprintf("{%s} in a hook command would be interpreted by the CLI's shell", m[1]),
				"a hook command may only use "+strings.Join(hookCommandVars, ", ")))
		}
	}
	return out
}

// hookCommands cuts every hook command out of a templated string: from each
// "{crowbar_hook} hook " to the quote that closes the string embedding it.
func hookCommands(s string) []string {
	var out []string
	for {
		start := strings.Index(s, "{crowbar_hook} hook ")
		if start < 0 {
			return out
		}
		s = s[start:]
		end := strings.IndexByte(s, '"')
		if end < 0 {
			end = len(s)
		}
		out = append(out, s[:end])
		s = s[end:]
	}
}

func versionRules(doc document, d *spec.Descriptor) []Finding {
	r := d.ProtocolVersion
	if r == nil {
		return nil
	}
	var out []Finding
	for key, v := range map[string]string{"min": r.Min, "max": r.Max} {
		if v != "" && !versionRE.MatchString(v) {
			out = append(out, doc.finding("version.range", SeverityError, "protocol_version."+key,
				fmt.Sprintf("%q is not a dotted numeric version", v), "write it as 0.156 or 0.156.1"))
		}
	}
	if len(out) == 0 && r.Min != "" && r.Max != "" && versionAfter(r.Min, r.Max) {
		out = append(out, doc.finding("version.range", SeverityError, "protocol_version",
			fmt.Sprintf("min %s is above max %s: no CLI version can pass", r.Min, r.Max), "swap or widen the range"))
	}
	return out
}

func versionAfter(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		av, bv := versionPart(as, i), versionPart(bs, i)
		if av != bv {
			return av > bv
		}
	}
	return false
}

func versionPart(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n := 0
	for _, c := range parts[i] {
		n = n*10 + int(c-'0')
	}
	return n
}
