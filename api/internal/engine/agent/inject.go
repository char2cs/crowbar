package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type SpawnPlan struct {
	Argv    []string
	Env     []string
	Cwd     string
	TmpDir  string
	Cleanup func()
}

// BuildSpawnPlan renders a descriptor's spawn.args + config_injection (+ any
// extraSteps such as session/handoff args) into a concrete argv/env/cwd, writing
// any hook-config files under ctx.Tmp. baseEnv is the process env to start from.
func BuildSpawnPlan(d *Descriptor, ctx TemplateCtx, baseEnv []string, extraSteps []InjectStep) (*SpawnPlan, error) {
	env := clearEnv(baseEnv, d.Spawn.Env.Clear)
	plan := &SpawnPlan{
		Cwd:     ctx.Cwd,
		TmpDir:  ctx.Tmp,
		Env:     env,
		Cleanup: func() { _ = os.RemoveAll(ctx.Tmp) },
	}
	// static spawn.args first (e.g. codex --dangerously-bypass-hook-trust)
	for _, a := range d.Spawn.Args {
		plan.Argv = append(plan.Argv, Expand(a, ctx))
	}
	steps := append([]InjectStep{}, d.ConfigInjection...)
	steps = append(steps, extraSteps...)
	for _, st := range steps {
		if err := runStep(d, st, ctx, plan); err != nil {
			plan.Cleanup()
			return nil, err
		}
	}
	// Hard guard (Global Constraints): the engine must never spawn a headless CLI.
	// Reject if any assembled argv token exactly equals a descriptor forbid_flag.
	for _, tok := range plan.Argv {
		for _, forbidden := range d.Spawn.ForbidFlags {
			if tok == forbidden {
				plan.Cleanup()
				return nil, fmt.Errorf("agent: forbidden flag %q for provider %q", tok, d.ID)
			}
		}
	}
	return plan, nil
}

func runStep(d *Descriptor, st InjectStep, ctx TemplateCtx, plan *SpawnPlan) error {
	arg := func(k string) string { return Expand(asString(st.Args[k]), ctx) }
	switch st.Verb {
	case "set_env":
		kv := arg("name") + "=" + arg("value")
		plan.Env = append(plan.Env, kv)
	case "write_file":
		return writeFileStep(arg("path"), arg("content"), arg("from"))
	case "render_hooks":
		return renderHooks(d, arg("into"), ctx)
	case "pass_arg":
		if pos, ok := st.Args["positional"]; ok {
			plan.Argv = append(plan.Argv, Expand(asString(pos), ctx))
			return nil
		}
		plan.Argv = append(plan.Argv, arg("arg"))
		if _, ok := st.Args["value"]; ok {
			plan.Argv = append(plan.Argv, arg("value"))
		}
	default:
		return fmt.Errorf("agent: unknown inject verb %q", st.Verb)
	}
	return nil
}

// renderHooks writes the provider hook config that maps each descriptor hook to
// `<crowbar_hook> hook <canonical-event>`. Both Claude settings.json and Codex
// hooks.json share the same nested shape {hooks:{Event:[{hooks:[{type,command}]}]}}.
func renderHooks(d *Descriptor, into string, ctx TemplateCtx) error {
	type cmd struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	}
	type group struct {
		Hooks []cmd `json:"hooks"`
	}
	events := map[string][]group{}
	for canonical, hm := range d.Hooks {
		command := ctx.CrowbarHook + " hook " + canonical
		events[hm.ProviderEvent] = []group{{Hooks: []cmd{{Type: "command", Command: command}}}}
	}
	payload := map[string]any{"hooks": events}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("agent: render hooks: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(into), 0o700); err != nil {
		return fmt.Errorf("agent: render hooks mkdir: %w", err)
	}
	return os.WriteFile(into, buf, 0o600)
}

func writeFileStep(path, content, from string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agent: write_file mkdir: %w", err)
	}
	if from != "" {
		src := expandHome(from)
		if _, err := os.Stat(src); err != nil {
			// Tolerate a missing optional source (e.g. ~/.codex/auth.json may not
			// exist yet) — write an empty destination rather than failing the build.
			return os.WriteFile(path, nil, 0o600)
		}
		return copyFile(src, path)
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func clearEnv(env, clear []string) []string {
	if len(clear) == 0 {
		return append([]string{}, env...)
	}
	drop := map[string]struct{}{}
	for _, k := range clear {
		drop[k] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if _, skip := drop[name]; skip {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("agent: copy open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("agent: copy create %s: %w", dst, err)
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
