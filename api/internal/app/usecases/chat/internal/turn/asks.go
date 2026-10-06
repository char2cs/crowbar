package turn

import (
	"context"
	"time"

	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/answerdesk"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// raiseAsk records a permission or elicitation the person must answer and holds
// its relay for them. It is the one path for every such ask, whichever
// conversation of the chat raised it.
func (t *Turns) raiseAsk(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
	now time.Time,
) {
	// Minted ONCE and shared: two draws on an empty PromptID would pair a
	// choice with an interruption that was never opened.
	cid := ""
	if ev.Choice != nil {
		cid = choiceID(ctx, chat.ID, ev.Choice)
	}
	iid := answerdesk.PermissionInterruptionID(cid)
	if iid == "" {
		iid = interruptionID(ctx, chat.ID, ev)
	}
	note(ctx, "interrupted", t.activity.Interrupt(
		ctx, chat.ID, iid, ev.Interrupt.Kind, ev.Interrupt.Detail, now,
	))

	t.openChoice(ctx, chat, runner, agent, ev, cid, raw, now)
}

func (t *Turns) openChoice(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	id string,
	raw []byte,
	now time.Time,
) {
	if ev.Choice == nil {
		return
	}
	chatID := chat.ID
	// A choice never durably opened in the ledger must never be held for a
	// human or auto-approved: both paths would act on a choice the ledger
	// never recorded, and the provider's own AnswerChoice call would reject
	// it as no longer pending. Falling through here leaves the CLI's own
	// native prompt as the only path, same as holdForAnswer's own silent
	// fallback for every other unanswerable-from-Crowbar reason.
	if err := t.activity.OpenChoice(ctx, agentactivity.ChoiceInput{
		ChatID:   chatID,
		ChoiceID: id,
		Kind:     ev.Choice.Kind,
		PromptID: ev.Choice.PromptID,
		ToolName: ev.Choice.ToolName,
		Title:    ev.Choice.Title,
		Question: ev.Choice.Question,
		Mode:     ev.Choice.Mode,
		Multi:    ev.Choice.Multi,
		Options:  choiceOptions(ev.Choice.Options),

		Questions: choiceQuestions(ev.Choice.Questions),
		Schema:    string(ev.Choice.Schema),
		Now:       now,
	}); err != nil {
		note(ctx, "choice opened", err)
		return
	}

	t.holdForAnswer(ctx, chat, runner, agent, ev, id, raw)
}
