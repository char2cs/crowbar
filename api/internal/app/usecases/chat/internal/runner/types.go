package runner

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type Conversations interface {
	ChatProviderID(
		ctx context.Context,
		chatID string,
	) (string, error)
	ChatSelection(
		ctx context.Context,
		chatID string,
	) (engineagents.Selection, error)
	SetChatSelection(
		ctx context.Context,
		chatID string,
		model, effort string,
	) error
	ChatTurns(
		ctx context.Context,
		chatID string,
	) ([]domain.LedgerTurn, error)
	AssembleConversation(
		ctx context.Context,
		chatID string,
		resuming bool,
		leftAt time.Time,
	) (string, error)
	ThreadContext(
		ctx context.Context,
		chatID string,
	) (string, error)
}

type AncestorCwd interface {
	ResolveCwdWorkspaceID(
		ctx context.Context,
		chatID string,
	) (string, bool, error)
}

type Turns interface {
	Hook(ctx context.Context, runnerID, provider, event string, raw []byte) (reply []byte, ask string, err error)
	ForgetChat(chatID string)
	ForgetTelemetry(ctx context.Context, chatID string)
	ExitRows(st ledger.State, chatID, runnerID string) ([]ledger.Row, error)
	LiveDiff(chatID, runnerID string) string
	Retire(chatID, runnerID string)

	termwait.Prompts
	termwait.Notices
	FailTurnOnNotice(ctx context.Context, runner engineagents.Runner, notice engineagents.TerminalNotice)
}

type Providers interface {
	RequireProviderEnabled(
		ctx context.Context,
		providerID string,
	) error
	ProviderMCPEnabled(
		ctx context.Context,
		providerID string,
	) (bool, error)
}
