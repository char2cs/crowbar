//go:build unix

package descriptorcheck

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var semverRE = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (l *live) binary() Step {
	path := binpath.Resolve(l.spec().Spawn.Cmd)
	if _, err := exec.LookPath(path); err != nil {
		return fail("binary", fmt.Sprintf("%s is not installed or not on PATH", l.spec().Spawn.Cmd))
	}
	return pass("binary", path)
}

func (l *live) version(ctx context.Context) Step {
	out, err := l.run(ctx, "--version")
	if err != nil {
		return fail("version", fmt.Sprintf("%s --version: %v", l.spec().Spawn.Cmd, err))
	}
	v := semverRE.FindString(out)
	if v == "" {
		return warn("version", "no version number in: "+tail(out, 120))
	}
	if err := protocol.CheckVersion(l.spec(), v); err != nil {
		return fail("version", err.Error())
	}
	return pass("version", versionLine(out, v))
}

// flags checks every long flag the descriptor passes against --help. Hidden
// flags exist, so a miss warns rather than fails.
func (l *live) flags(ctx context.Context) Step {
	help, err := l.run(ctx, "--help")
	if err != nil {
		return warn("flags", "--help failed: "+err.Error())
	}
	var missing []string
	for _, flag := range declaredFlags(l.spec()) {
		if !strings.Contains(help, flag) {
			missing = append(missing, flag)
		}
	}
	if len(missing) > 0 {
		return warn("flags", "not in --help (hidden, or renamed?): "+strings.Join(missing, " "))
	}
	return pass("flags", "every declared flag is documented")
}

// versionLine is the output line naming the version: a CLI may print
// warnings first.
func versionLine(out, v string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, v) {
			return strings.TrimSpace(line)
		}
	}
	return v
}

var longFlagRE = regexp.MustCompile(`^--[a-z0-9][a-z0-9-]*`)

func declaredFlags(d *spec.Descriptor) []string {
	seen := map[string]bool{}
	var out []string
	add := func(tok string) {
		if f := longFlagRE.FindString(tok); f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, a := range d.Spawn.Args {
		add(a)
	}
	if d.Session.Resume != nil {
		for _, tok := range strings.Fields(d.Session.Resume.Arg) {
			add(tok)
		}
	}
	for _, name := range sortedKeys(injections(d)) {
		for _, step := range injections(d)[name] {
			add(spec.ArgString(step.Args["arg"]))
		}
	}
	return out
}

func (l *live) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binpath.Resolve(l.spec().Spawn.Cmd), args...) //nolint:gosec // the descriptor's own CLI, run on purpose
	cmd.Env = l.opts.Env
	cmd.Dir = l.cwd
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, tail(string(out), 200))
	}
	return string(out), nil
}

// resumeUnknown launches a resume of a session that never existed. The CLI
// must end: the resume probe reads an early exit as "that session is gone".
func (l *live) resumeUnknown(ctx context.Context) Step {
	arg, ok := l.agent.ResumeArg()
	if !ok {
		return skip("resume_unknown", "the descriptor declares no session.resume")
	}
	tmp, err := l.tmpDir("resume")
	if err != nil {
		return fail("resume_unknown", err.Error())
	}
	id := "00000000-0000-4000-8000-00000000c0de"
	var steps []agents.InjectStep
	for _, part := range strings.Fields(agents.Expand(arg, agents.TemplateCtx{ID: id})) {
		steps = append(steps, agents.InjectStep{Verb: "pass_arg", Args: map[string]any{"positional": part}})
	}
	t, err := l.spawn(ctx, tmp, steps)
	if err != nil {
		return fail("resume_unknown", err.Error())
	}
	defer t.close()
	deadline := time.Now().Add(l.opts.ResumeWindow)
	for time.Now().Before(deadline) {
		if ended, code := t.exited(pollEvery); ended {
			return pass("resume_unknown", fmt.Sprintf("exited %d for an unknown session", code))
		}
		if prompt, ok := l.agent.MatchTerminalPrompt(t.text()); ok {
			return warn("resume_unknown", fmt.Sprintf(
				"parked on a %s prompt before resuming (first-run setup, login or folder trust); log the CLI in once, or rerun with --cwd set to a directory it trusts", promptName(prompt.Kind)))
		}
	}
	return fail("resume_unknown", fmt.Sprintf(
		"still running after %s on an unknown session; a lost session would look live: %s",
		l.opts.ResumeWindow, tail(t.text(), 160)))
}

func (l *live) spawn(ctx context.Context, tmp string, steps []agents.InjectStep) (*term, error) {
	plan, err := l.agent.SpawnPlan(l.templateCtx(tmp), l.opts.Env, steps)
	if err != nil {
		return nil, fmt.Errorf("spawn plan: %w", err)
	}
	return startTerm(ctx, append([]string{plan.Executable}, plan.Argv...), plan.Env, l.cwd)
}

func (l *live) spec() *spec.Descriptor { return l.d }
