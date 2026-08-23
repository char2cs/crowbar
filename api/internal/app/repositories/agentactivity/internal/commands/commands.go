package commands

import (
	"fmt"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func advance(current *domain.AgentActivity, chatID string) domain.AgentActivity {
	if current == nil {
		return domain.AgentActivity{ChatID: chatID, Seq: 1}
	}
	next := *current
	next.ChatID = chatID
	next.Seq = current.Seq + 1
	next.Last = nil
	next.Tools = cloneTools(current.Tools)
	next.Subagents = cloneSubagents(current.Subagents)
	next.Interruptions = cloneInterruptions(current.Interruptions)
	next.Choices = cloneChoices(current.Choices)
	return next
}

func cloneTools(in map[string]domain.ActivityToolCall) map[string]domain.ActivityToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivityToolCall, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneSubagents(in map[string]domain.ActivitySubagent) map[string]domain.ActivitySubagent {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivitySubagent, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneInterruptions(in map[string]domain.ActivityInterruption) map[string]domain.ActivityInterruption {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivityInterruption, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneChoices(in map[string]domain.ActivityChoice) map[string]domain.ActivityChoice {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]domain.ActivityChoice, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func requireChat(op, chatID string) error {
	if chatID == "" {
		return fmt.Errorf("%s: missing chat id: %w", op, asynxModels.ErrValidation)
	}
	return nil
}

func requireID(op, name, id string) error {
	if id == "" {
		return fmt.Errorf("%s: missing %s: %w", op, name, asynxModels.ErrValidation)
	}
	return nil
}

func full(a *domain.AgentActivity) bool {
	return a != nil && a.OpenCount() >= domain.MaxOpenPerTurn
}

func at(t time.Time) *time.Time {
	v := t
	return &v
}
