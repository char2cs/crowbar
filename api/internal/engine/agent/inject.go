package agent

import (
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

// BuildSpawnPlan renders a descriptor's spawn.args + mcp_injection +
// config_injection (+ any extraSteps such as session/handoff args) into a
// concrete argv/env/cwd, writing any hook-config files under ctx.Tmp. baseEnv is
// the process env to start from.
//
// It renders whatever MCPInject the descriptor it is HANDED declares. Whether
// this provider's tool surface should be registered at all is a user preference,
// so the decision is the usecase's and arrives here as a descriptor with the
// field already emptied — the engine has no access to a preference table and
// should not grow one.
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
	// mcp_injection BEFORE config_injection, so a descriptor can guarantee with its
	// own steps what follows its MCP registration. claude's --mcp-config is
	// variadic and swallows any bare positional after it (see claude.yaml), and
	// what stops that is the --settings pair sitting immediately behind it in
	// config_injection. Rendering the MCP steps after config_injection instead
	// would put the JSON one token away from a resumed session's id.
	steps := append([]InjectStep{}, d.MCPInject...)
	steps = append(steps, d.ConfigInjection...)
	steps = append(steps, extraSteps...)
	for _, st := range steps {
		if err := runStep(st, ctx, plan); err != nil {
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

func runStep(st InjectStep, ctx TemplateCtx, plan *SpawnPlan) error {
	arg := func(k string) string { return Expand(asString(st.Args[k]), ctx) }
	switch st.Verb {
	case "set_env":
		plan.Env = append(plan.Env, arg("name")+"="+arg("value"))
	case "write_file":
		return writeFileStep(arg("path"), arg("content"), arg("from"))
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

func writeFileStep(path, content, from string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("agent: write_file mkdir: %w", err)
	}
	if from != "" {
		src := expandHome(from)
		if _, err := os.Stat(src); err != nil {
			// Tolerate a missing optional source — write an empty destination rather than
			// failing the spawn. (No descriptor ships a `from:` today, and none should
			// point one at a credential: a provider owns its own secrets and Crowbar
			// copies none of them.)
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
