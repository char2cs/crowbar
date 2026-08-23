package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/termprompt"
)

type TerminalNotice = models.TerminalNotice

const (
	TerminalNoticeUsageLimit = spec.TerminalNoticeUsageLimit
)

type NoticeMatcher interface {
	MatchTerminalNotice(screen string) (TerminalNotice, bool)
}

func (a *agent) MatchTerminalNotice(screen string) (TerminalNotice, bool) {
	return termprompt.MatchNotice(a.spec, screen)
}

var _ NoticeMatcher = (*agent)(nil)
