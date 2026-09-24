package descriptorcheck

import (
	"context"
	"os"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
)

// Status is the outcome of one conformance step.
type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Step is one conformance check against the real CLI.
type Step struct {
	Name    string        `json:"name"`
	Status  Status        `json:"status"`
	Detail  string        `json:"detail,omitempty"`
	Elapsed time.Duration `json:"elapsedNs"`
}

// LiveReport is a descriptor's static report plus its live conformance steps.
type LiveReport struct {
	Report
	Steps []Step `json:"steps"`
}

// OK reports whether nothing failed, statically or live.
func (r LiveReport) OK() bool {
	if !r.Report.OK() {
		return false
	}
	for _, s := range r.Steps {
		if s.Status == StatusFail {
			return false
		}
	}
	return true
}

// LiveOptions tunes a conformance run.
type LiveOptions struct {
	// Cwd is the directory the CLI boots in; a fresh temporary one if empty.
	Cwd string
	// Turn runs one real model turn (one billed request) to prove the hook
	// round trip for a provider that reports hooks only once a turn starts.
	Turn bool
	// Prompt is the turn's message.
	Prompt string
	// BootWindow is how long a TUI must stay up to count as booted.
	BootWindow time.Duration
	// ResumeWindow bounds how long an unknown-session resume may run.
	ResumeWindow time.Duration
	// TurnTimeout bounds the model turn.
	TurnTimeout time.Duration
	// ServeWindow bounds the app-server's start and its initialize answer.
	ServeWindow time.Duration
	// Env is the CLI's base environment; the daemon's own if nil.
	Env []string
}

func (o LiveOptions) withDefaults() LiveOptions {
	if o.BootWindow == 0 {
		o.BootWindow = 8 * time.Second
	}
	if o.ResumeWindow == 0 {
		o.ResumeWindow = 30 * time.Second
	}
	if o.TurnTimeout == 0 {
		o.TurnTimeout = 3 * time.Minute
	}
	if o.ServeWindow == 0 {
		o.ServeWindow = 20 * time.Second
	}
	if o.Prompt == "" {
		o.Prompt = "Reply with the single word: ok"
	}
	if o.Env == nil {
		o.Env = os.Environ()
	}
	return o
}

// Conform validates raw and, when nothing static blocks it, runs it against
// the real CLI: binary, version, flags, TUI boot, hooks, resume, app-server.
func Conform(ctx context.Context, raw []byte, opts LiveOptions) LiveReport {
	rep := LiveReport{Report: Validate(raw)}
	if !rep.Report.OK() {
		rep.Steps = []Step{{Name: "static", Status: StatusFail, Detail: "fix the static findings first"}}
		return rep
	}
	d, _ := protocol.CheckDescriptor(raw)
	rep.Steps = runLive(ctx, raw, d, opts.withDefaults())
	return rep
}
