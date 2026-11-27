package chat

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/runner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const WaitingForPendingPromptLog = runner.WaitingForPendingPromptLog

func HasPendingPrompt(u RunnerUsecase, ctx context.Context, chatID string) bool {
	st, err := concrete(u).ledger.State(ctx, chatID)
	return err == nil && len(st.PendingPrompts()) > 0
}

func ChatTurns(u RunnerUsecase, ctx context.Context, chatID string) ([]domain.LedgerTurn, error) {
	return concrete(u).conversations.ChatTurns(ctx, chatID)
}

func ChatExitReason(u RunnerUsecase, chatID string) string {
	snap, err := concrete(u).ChatSnapshot(context.Background(), chatID)
	if err != nil {
		panic(err)
	}
	if snap.Ledger.LastExit == nil {
		return ""
	}
	return snap.Ledger.LastExit.Reason
}

func concrete(u RunnerUsecase) *Usecase {
	uc, ok := u.(*Usecase)
	if !ok {
		panic(fmt.Sprintf("chat: %T is not the chat usecase", u))
	}
	return uc
}

func AllLiveRunners(u *Usecase) []engineagents.Runner {
	all, err := u.runners.AllLive(context.Background())
	if err != nil {
		panic(err)
	}
	return all
}

func RequirePromptRestart(
	ctx context.Context,
	u RunnerUsecase,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	return u.(*Usecase).runners.RequirePromptRestart(ctx, chatID, live, descriptor)
}
