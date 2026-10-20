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
// its relay for them, returning the ask it is held on. It is the one path for
// every such ask, whichever conversation of the chat raised it.
func (t *Turns) raiseAsk(
	ctx context.Context,
	chat domain.Chat,
	runner engineagents.Runner,
	agent engineagents.Agent,
	ev engineagents.CanonicalEvent,
	raw []byte,
	now time.Time,
) (string, error) {
	// Minted ONCE and shared: two draws on an empty PromptID would pair a
	// choice with an interruption that was never opened.
	cid := ""
	if ev.Choice != nil {
		cid = choiceID(chat.ID, ev.Choice)
	}
	iid := answerdesk.PermissionInterruptionID(cid)
	if iid == "" {
		iid = interruptionID(chat.ID, ev)
	}
	if err := t.activity.Interrupt(ctx, chat.ID, iid, ev.Interrupt.Kind, ev.Interrupt.Detail, now); err != nil {
		return "", wrote("interrupted", err)
	}
	return t.openChoice(ctx, chat, runner, agent, ev, cid, raw, now)
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
) (string, error) {
	if ev.Choice == nil {
		return "", nil
	}
	chatID := chat.ID
	// A choice the record refused is never held: an answer would act on a choice
	// it does not have. The hook is refused, which leaves the CLI its own prompt.
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
		return "", wrote("choice opened", err)
	}
	return t.holdForAnswer(ctx, chat, runner, agent, ev, id, raw), nil
}
