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

func KnownTurnRole(role string) bool {
	switch role {
	case TurnRoleUser, TurnRoleAssistant, TurnRoleHarness, TurnRoleNotice:
		return true
	default:
		return false
	}
}

const (
	ToolStatusRunning   = "running"
	ToolStatusOK        = "ok"
	ToolStatusError     = "error"
	ToolStatusAbandoned = "abandoned"
)

const MaxOpenPerTurn = 64

type ChatActivity struct {
	ChatID string `json:"chatId"`

	Seq int64 `json:"seq"`

	Turn *ActivityTurn `json:"turn,omitempty"`

	Tools         map[string]ActivityToolCall     `json:"tools,omitempty"`
	Subagents     map[string]ActivitySubagent     `json:"subagents,omitempty"`
	Interruptions map[string]ActivityInterruption `json:"interruptions,omitempty"`

	Choices map[string]ActivityChoice `json:"choices,omitempty"`

	Last *ActivityDelta `json:"last,omitempty"`
}

const (
	DeltaOpen  = "open"
	DeltaClose = "close"

	DeltaTurn         = "turn"
	DeltaTool         = "tool"
	DeltaSubagent     = "subagent"
	DeltaInterruption = "interruption"
	DeltaChoice       = "choice"
)

type ActivityDelta struct {
	Phase string `json:"phase"`
	Kind  string `json:"kind"`

	SupersededTurnID string `json:"supersededTurnId,omitempty"`

	Turn         *ActivityTurn         `json:"turn,omitempty"`
	Tool         *ActivityToolCall     `json:"tool,omitempty"`
	Subagent     *ActivitySubagent     `json:"subagent,omitempty"`
	Interruption *ActivityInterruption `json:"interruption,omitempty"`
	Choice       *ActivityChoice       `json:"choice,omitempty"`
}

type ActivityTurn struct {
	ID         string `json:"id"`
	ChatID     string `json:"chatId"`
	Seq        int64  `json:"seq"`
	Role       string `json:"role"`
	ProviderID string `json:"providerId"`
	RunnerID   string `json:"runnerId"`
	SessionID  string `json:"sessionId"`

	Text   string `json:"text"`
	Effort string `json:"effort,omitempty"`

	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type ActivityToolCall struct {
	ID     string `json:"id"`
	TurnID string `json:"turnId"`
	ChatID string `json:"chatId"`
	Seq    int64  `json:"seq"`

	Name string `json:"name"`

	Target string `json:"target,omitempty"`

	RequestRef string `json:"requestRef,omitempty"`
	ResultRef  string `json:"resultRef,omitempty"`

	Status string `json:"status"`

	Error      string `json:"error,omitempty"`
	DurationMS int    `json:"durationMs,omitempty"`

	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type ActivitySubagent struct {
	ID        string     `json:"id"`
	TurnID    string     `json:"turnId"`
	ChatID    string     `json:"chatId"`
	Seq       int64      `json:"seq"`
	AgentType string     `json:"agentType,omitempty"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type ActivityInterruption struct {
	ID         string     `json:"id"`
	TurnID     string     `json:"turnId"`
	ChatID     string     `json:"chatId"`
	Seq        int64      `json:"seq"`
	Kind       string     `json:"kind"`
	Detail     string     `json:"detail,omitempty"`
	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

func (a ChatActivity) OpenCount() int {
	return len(a.Tools) + len(a.Subagents) + len(a.Interruptions) + len(a.Choices)
}

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
	ChoiceResolutionAnswered = "answered"

	ChoiceResolutionProceeded = "proceeded"

	ChoiceResolutionAbandoned = "abandoned"
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

	Title    string                 `json:"title,omitempty"`
	Question string                 `json:"question,omitempty"`
	Mode     string                 `json:"mode,omitempty"`
	Multi    bool                   `json:"multi,omitempty"`
	Options  []ActivityChoiceOption `json:"options,omitempty"`

	Questions []ActivityChoiceQuestion `json:"questions,omitempty"`

	Schema string `json:"schema,omitempty"`

	At         time.Time  `json:"at"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
	Resolution string     `json:"resolution,omitempty"`
	// AutoApproved distinguished a policy-decided answer from a human's own
	// click. Crowbar no longer decides permissions itself — every provider's
	// own native mode does (PermissionLevel), so every answer that reaches
	// here is a human's — and this is currently always false. Kept on the
	// wire rather than removed outright: a client reading it still gets a
	// truthful answer, just a constant one.
	AutoApproved bool `json:"autoApproved,omitempty"`
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
}

func (c ActivityChoice) Pending() bool { return c.ResolvedAt == nil }

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
