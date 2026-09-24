//go:build unix

package descriptorcheck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// live is one conformance run: a private crowbar home holding the descriptor
// under test, a working directory, and a hook recorder standing in for
// `crowbar hook`.
type live struct {
	opts  LiveOptions
	d     *spec.Descriptor
	id    string
	agent agents.Agent
	svc   agents.Agents

	home, cwd, scratch string
	hooks              recorder
}

func runLive(ctx context.Context, raw []byte, d *spec.Descriptor, opts LiveOptions) []Step {
	l, err := newLive(ctx, raw, d, opts)
	if err != nil {
		return []Step{{Name: "setup", Status: StatusFail, Detail: err.Error()}}
	}
	defer l.close()

	steps := []Step{timed(l.binary)}
	if steps[0].Status == StatusFail {
		return steps
	}
	steps = append(steps, timed(func() Step { return l.version(ctx) }), timed(func() Step { return l.flags(ctx) }))
	steps = append(steps, l.boot(ctx)...)
	steps = append(steps, timed(func() Step { return l.resumeUnknown(ctx) }))
	return append(steps, timed(func() Step { return l.appServer(ctx) }))
}

func newLive(ctx context.Context, raw []byte, d *spec.Descriptor, opts LiveOptions) (*live, error) {
	// Short: a unix socket path is capped near 104 bytes on macOS.
	scratch, err := os.MkdirTemp("", "cbconf")
	if err != nil {
		return nil, fmt.Errorf("scratch dir: %w", err)
	}
	l := &live{opts: opts, d: d, id: d.ID, scratch: scratch, home: filepath.Join(scratch, "home")}
	if err := l.prepare(raw); err != nil {
		l.close()
		return nil, err
	}
	l.svc = agents.New()
	l.agent, err = l.svc.Get(ctx, l.home, d.ID)
	if err != nil {
		l.close()
		return nil, fmt.Errorf("load descriptor: %w", err)
	}
	return l, nil
}

func (l *live) prepare(raw []byte) error {
	dir := filepath.Join(l.home, "descriptors")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("home: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, l.id+".yaml"), raw, 0o600); err != nil {
		return fmt.Errorf("descriptor: %w", err)
	}
	l.cwd = l.opts.Cwd
	if l.cwd == "" {
		l.cwd = filepath.Join(l.scratch, "work")
		if err := os.MkdirAll(l.cwd, 0o750); err != nil {
			return fmt.Errorf("work dir: %w", err)
		}
	}
	hooks, err := newRecorder(l.scratch)
	if err != nil {
		return err
	}
	l.hooks = hooks
	return nil
}

func (l *live) close() {
	if l.svc != nil {
		l.svc.Close()
	}
	_ = os.RemoveAll(l.scratch)
}

// templateCtx is what the runner fills for a spawn, with the recorder as the
// hook relay.
func (l *live) templateCtx(tmp string) agents.TemplateCtx {
	return agents.TemplateCtx{
		Tmp: tmp, Cwd: l.cwd, CrowbarHook: l.hooks.path, CrowbarHome: l.home,
		Segid: "conformance", Provider: l.id, ProjectID: "conformance", WorkspaceID: "conformance",
		ChatID: "conformance", RunnerToken: "conformance",
	}
}

func (l *live) tmpDir(name string) (string, error) {
	dir := filepath.Join(l.scratch, name)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("tmp dir: %w", err)
	}
	return dir, nil
}

func timed(f func() Step) Step {
	start := time.Now()
	s := f()
	s.Elapsed = time.Since(start)
	return s
}

func pass(name, detail string) Step { return Step{Name: name, Status: StatusPass, Detail: detail} }
func warn(name, detail string) Step { return Step{Name: name, Status: StatusWarn, Detail: detail} }
func fail(name, detail string) Step { return Step{Name: name, Status: StatusFail, Detail: detail} }
func skip(name, detail string) Step { return Step{Name: name, Status: StatusSkip, Detail: detail} }
