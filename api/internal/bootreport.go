package internal

import (
	"fmt"
	"sync/atomic"

	"github.com/char2cs/crowbar/api/internal/core/bootgate"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

const startingMessage = "Crowbar is starting"

type bootReport struct {
	session atomic.Pointer[migrate.Session]
}

type progressDetail struct {
	Phase string     `json:"phase"`
	Step  migrate.ID `json:"step"`
	Index int        `json:"index"`
	Total int        `json:"total"`
}

func (b *bootReport) report() bootgate.Report {
	s := b.session.Load()
	if s == nil {
		return bootgate.Report{State: bootgate.StateMigrating, Message: startingMessage}
	}
	if h := s.Halt(); h != nil {
		return bootgate.Report{State: bootgate.StateHalted, Message: h.Message, Detail: h}
	}
	p := s.Progress()
	if p.Total == 0 {
		return bootgate.Report{State: bootgate.StateMigrating, Message: startingMessage}
	}
	return bootgate.Report{
		State:   bootgate.StateMigrating,
		Message: fmt.Sprintf("Crowbar is upgrading its data (step %d of %d: %s)", p.Index, p.Total, p.Step.Name),
		Detail:  progressDetail{Phase: p.Phase.String(), Step: p.Step, Index: p.Index, Total: p.Total},
	}
}
