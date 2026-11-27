package descriptorcheck

import (
	"context"
	"os"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
)

type Status string

const (
	StatusPass Status = "pass"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

type Step struct {
	Name    string        `json:"name"`
	Status  Status        `json:"status"`
	Detail  string        `json:"detail,omitempty"`
	Elapsed time.Duration `json:"elapsedNs"`
}

type LiveReport struct {
	Report
	Steps []Step `json:"steps"`
}

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

type LiveOptions struct {
	Cwd          string
	Turn         bool
	Prompt       string
	BootWindow   time.Duration
	ResumeWindow time.Duration
	TurnTimeout  time.Duration
	ServeWindow  time.Duration
	Env          []string
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
