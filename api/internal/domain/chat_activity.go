package domain

import (
	"fmt"
	"time"
)

const (
	TurnRoleUser = "user"

	TurnRoleAssistant = "assistant"

	TurnRoleHarness = "harness"

	TurnRoleNotice = "notice"
)

const (
	ToolStatusOK       = "ok"
	ToolStatusError    = "error"
	ToolStatusDeclined = "declined"
)

const (
	ChoiceKindPermission  = "tool_permission"
	ChoiceKindQuestion    = "question"
	ChoiceKindElicitation = "elicitation"
)

const (
	ChoiceOptionAnswer     = "answer"
	ChoiceOptionAllow      = "allow"
	ChoiceOptionDeny       = "deny"
	ChoiceOptionSuggestion = "suggestion"
)

const (
	ChoiceResolutionProceeded = "proceeded"
	ChoiceResolutionRefused   = "refused"
)

type ActivityChoice struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	ChatID string `json:"chatId"`
	Seq    int64  `json:"seq"`

	Kind string `json:"kind"`

	PromptID string `json:"promptId,omitempty"`

	ToolID   string `json:"toolId,omitempty"`
	ToolName string `json:"toolName,omitempty"`
	// ToolTarget and MCPServer say what the call it asks about acts on.
	ToolTarget string `json:"toolTarget,omitempty"`
	MCPServer  string `json:"mcpServer,omitempty"`

	Title    string                 `json:"title,omitempty"`
	Question string                 `json:"question,omitempty"`
	Mode     string                 `json:"mode,omitempty"`
	Multi    bool                   `json:"multi,omitempty"`
	Options  []ActivityChoiceOption `json:"options,omitempty"`

	Questions []ActivityChoiceQuestion `json:"questions,omitempty"`

	Schema string `json:"schema,omitempty"`

	At time.Time `json:"at"`
}

type ActivityChoiceQuestion struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`

	Text    string                 `json:"text,omitempty"`
	Multi   bool                   `json:"multi,omitempty"`
	Options []ActivityChoiceOption `json:"options,omitempty"`
}

type ActivityChoiceOption struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	// Reason: answering with this option sends the person's reason to the agent.
	Reason bool `json:"reason,omitempty"`
}

type ChoiceAnswer struct {
	Question ActivityChoiceQuestion
	Picked   []ActivityChoiceOption
}

func (c ActivityChoice) AskedQuestions() []ActivityChoiceQuestion {
	if len(c.Questions) > 0 {
		return c.Questions
	}
	if len(c.Options) == 0 {
		return nil
	}
	question := ActivityChoiceQuestion{ID: "q0", Multi: c.Multi, Options: c.Options}
	if c.Kind == ChoiceKindQuestion {
		question.Title, question.Text = c.Title, c.Question
	}
	return []ActivityChoiceQuestion{question}
}

func (c ActivityChoice) ResolvePicks(optionIDs []string) ([]ChoiceAnswer, error) {
	questions := c.AskedQuestions()
	if len(questions) == 0 {
		return nil, nil
	}

	answers := make([]ChoiceAnswer, len(questions))
	for i, question := range questions {
		answers[i] = ChoiceAnswer{Question: question}
	}
	seen := make(map[string]bool, len(optionIDs))
	for _, id := range optionIDs {
		at, option, offered := findOption(questions, id)
		if !offered {
			return nil, fmt.Errorf("%q is not an option on this prompt", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("%q was picked more than once", id)
		}
		seen[id] = true
		answers[at].Picked = append(answers[at].Picked, option)
	}

	for _, answer := range answers {
		switch {
		case len(answer.Picked) == 0:
			return nil, fmt.Errorf("%s has no answer, and every question must be answered",
				questionName(answer.Question))
		case len(answer.Picked) > 1 && !answer.Question.Multi:
			return nil, fmt.Errorf("%s takes one answer, got %d",
				questionName(answer.Question), len(answer.Picked))
		}
	}
	return answers, nil
}

func findOption(
	questions []ActivityChoiceQuestion,
	id string,
) (int, ActivityChoiceOption, bool) {
	for i, question := range questions {
		for _, option := range question.Options {
			if option.ID == id {
				return i, option, true
			}
		}
	}
	return 0, ActivityChoiceOption{}, false
}

func questionName(q ActivityChoiceQuestion) string {
	switch {
	case q.Text != "":
		return fmt.Sprintf("%q", q.Text)
	case q.Title != "":
		return fmt.Sprintf("%q", q.Title)
	default:
		return "this prompt"
	}
}

func (q ActivityChoiceQuestion) AnswerKey() string {
	if q.Text != "" {
		return q.Text
	}
	return q.Title
}
