package commands

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type OpenChoice struct {
	ChatID    string
	ChoiceID  string
	Kind      string
	PromptID  string
	ToolName  string
	Title     string
	Question  string
	Mode      string
	Multi     bool
	Options   []domain.ActivityChoiceOption
	Questions []domain.ActivityChoiceQuestion
	Schema    string
	Now       time.Time
}

func (c OpenChoice) AggregateID() string  { return c.ChatID }
func (c OpenChoice) EventName() string    { return "agentactivity.choice_opened." + c.ChatID }
func (c OpenChoice) ShouldSnapshot() bool { return false }

func (c OpenChoice) Validate(*domain.AgentActivity) error {
	if err := requireChat("open choice", c.ChatID); err != nil {
		return err
	}
	if err := requireID("open choice", "choice id", c.ChoiceID); err != nil {
		return err
	}
	return requireID("open choice", "kind", c.Kind)
}

func (c OpenChoice) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)

	item := domain.ActivityChoice{
		ID:        c.ChoiceID,
		ChatID:    c.ChatID,
		Seq:       next.Seq,
		Kind:      c.Kind,
		PromptID:  c.PromptID,
		ToolName:  c.ToolName,
		Title:     c.Title,
		Question:  c.Question,
		Mode:      c.Mode,
		Multi:     c.Multi,
		Options:   c.Options,
		Questions: c.Questions,
		Schema:    c.Schema,
		At:        c.Now,
	}

	if next.Turn == nil {
		item.ResolvedAt = at(c.Now)
		item.Resolution = domain.ChoiceResolutionAbandoned
		next.Last = &domain.ActivityDelta{
			Phase: domain.DeltaClose, Kind: domain.DeltaChoice, Choice: &item,
		}
		return next
	}

	item.TurnID = next.Turn.ID
	item.ToolID = gatedTool(&next, c.ToolName)
	if !full(&next) {
		if next.Choices == nil {
			next.Choices = map[string]domain.ActivityChoice{}
		}
		next.Choices[c.ChoiceID] = item
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaOpen, Kind: domain.DeltaChoice, Choice: &item,
	}
	return next
}

func gatedTool(a *domain.AgentActivity, toolName string) string {
	if toolName == "" {
		return ""
	}
	found := ""
	var newest int64
	for id, call := range a.Tools {
		if call.Name != toolName || call.Seq < newest {
			continue
		}
		found, newest = id, call.Seq
	}
	return found
}

func dropChoicesForTool(a *domain.AgentActivity, toolID, toolName string) {
	for id, choice := range a.Choices {
		if gatesTool(choice, toolID, toolName) {
			delete(a.Choices, id)
		}
	}
	if len(a.Choices) == 0 {
		a.Choices = nil
	}
}

func gatesTool(choice domain.ActivityChoice, toolID, toolName string) bool {
	if choice.ToolID != "" {
		return choice.ToolID == toolID
	}
	return toolName != "" && choice.ToolName == toolName
}

type ResolveChoice struct {
	ChatID     string
	ChoiceID   string
	Resolution string
	Now        time.Time
}

func (c ResolveChoice) AggregateID() string  { return c.ChatID }
func (c ResolveChoice) EventName() string    { return "agentactivity.choice_resolved." + c.ChatID }
func (c ResolveChoice) ShouldSnapshot() bool { return false }

func (c ResolveChoice) Validate(*domain.AgentActivity) error {
	if err := requireChat("resolve choice", c.ChatID); err != nil {
		return err
	}
	return requireID("resolve choice", "choice id", c.ChoiceID)
}

func (c ResolveChoice) EmitEvent(current *domain.AgentActivity) domain.AgentActivity {
	next := advance(current, c.ChatID)
	item, known := next.Choices[c.ChoiceID]
	if !known {
		return next
	}
	delete(next.Choices, c.ChoiceID)
	if len(next.Choices) == 0 {
		next.Choices = nil
	}
	item.ResolvedAt = at(c.Now)
	item.Resolution = c.Resolution
	if item.Resolution == "" {
		item.Resolution = domain.ChoiceResolutionAnswered
	}
	next.Last = &domain.ActivityDelta{
		Phase: domain.DeltaClose, Kind: domain.DeltaChoice, Choice: &item,
	}
	return next
}
