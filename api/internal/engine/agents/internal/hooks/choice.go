package hooks

import (
	"strconv"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
)

const maxChoiceOptions = 32

const maxChoiceQuestions = 32

const maxChoiceSchemaBytes = 8 << 10

const (
	suggestionLabelPrefix  = "suggestion_label."
	suggestionLabelDefault = suggestionLabelPrefix + "default"
)

var choiceFields = [...]string{
	"prompt_id", "tool_name", "tool_input", "questions", "suggestions", "suggestion_type",
}

func declaresChoice(fields map[string]string) bool {
	for _, name := range choiceFields {
		if fields[name] != "" {
			return true
		}
	}
	return false
}

func permissionChoice(fields map[string]string, decoded map[string]any) *models.ChoicePrompt {
	if !declaresChoice(fields) {
		return nil
	}
	promptID := firstNonEmpty(decoded, fields["prompt_id"])
	toolName := firstNonEmpty(decoded, fields["tool_name"])

	if questions := payload.Objects(decoded, fields["questions"]); len(questions) > 0 {
		return questionChoice(fields, questions, promptID, toolName)
	}

	prompt := &models.ChoicePrompt{
		Kind:     models.ChoiceToolPermission,
		PromptID: promptID,
		ToolName: toolName,
		Title:    toolName,

		Options: []models.ChoiceOption{
			{ID: models.ChoiceOptionAllow, Kind: models.ChoiceOptionAllow, Label: "Allow"},
			{ID: models.ChoiceOptionDeny, Kind: models.ChoiceOptionDeny, Label: "Deny"},
		},
	}
	prompt.Options = append(prompt.Options, suggestionOptions(fields, decoded)...)
	return prompt
}

func questionChoice(
	fields map[string]string,
	questions []map[string]any,
	promptID, toolName string,
) *models.ChoicePrompt {
	prompt := &models.ChoicePrompt{
		Kind:     models.ChoiceQuestion,
		PromptID: promptID,
		ToolName: toolName,
	}
	if len(questions) > maxChoiceQuestions {
		return prompt
	}
	for i, question := range questions {
		prompt.Questions = append(prompt.Questions, choiceQuestion(fields, question, i))
	}
	if len(prompt.Questions) == 1 {
		prompt.Title = prompt.Questions[0].Title
		prompt.Question = prompt.Questions[0].Text
	}
	return prompt
}

func choiceQuestion(
	fields map[string]string,
	question map[string]any,
	index int,
) models.PromptQuestion {
	multi, _ := payload.Bool(question, fields["question_multi"])
	id := "q" + strconv.Itoa(index)
	out := models.PromptQuestion{
		ID:    id,
		Title: firstNonEmpty(question, fields["question_title"]),
		Text:  firstNonEmpty(question, fields["question_text"]),
		Multi: multi,
	}
	for i, option := range payload.Objects(question, fields["question_options"]) {
		if len(out.Options) >= maxChoiceOptions {
			break
		}
		out.Options = append(out.Options, models.ChoiceOption{
			ID:          id + "-answer-" + strconv.Itoa(i),
			Kind:        models.ChoiceOptionAnswer,
			Label:       firstNonEmpty(option, fields["option_label"]),
			Description: firstNonEmpty(option, fields["option_description"]),
		})
	}
	return out
}

func suggestionOptions(fields map[string]string, decoded map[string]any) []models.ChoiceOption {
	suggestions := payload.Objects(decoded, fields["suggestions"])
	out := make([]models.ChoiceOption, 0, len(suggestions))
	for i, suggestion := range suggestions {
		if len(out) >= maxChoiceOptions {
			break
		}
		label := suggestionLabel(fields, suggestion)
		if label == "" {
			continue
		}
		out = append(out, models.ChoiceOption{
			ID:          "suggestion-" + strconv.Itoa(i),
			Kind:        models.ChoiceOptionSuggestion,
			Label:       label,
			Description: firstNonEmpty(suggestion, fields["suggestion_description"]),
		})
	}
	return out
}

func suggestionLabel(fields map[string]string, suggestion map[string]any) string {
	kind := firstNonEmpty(suggestion, fields["suggestion_type"])
	if kind != "" {
		if label := fields[suggestionLabelPrefix+kind]; label != "" {
			return label
		}
	}

	return fields[suggestionLabelDefault]
}

func elicitationChoice(
	fields map[string]string,
	decoded map[string]any,
	message string,
) *models.ChoicePrompt {
	return &models.ChoicePrompt{
		Kind:     models.ChoiceElicitation,
		Title:    firstNonEmpty(decoded, fields["mcp_server"]),
		Question: message,
		Mode:     firstNonEmpty(decoded, fields["mode"]),
		Schema:   boundedSchema(decoded, fields["schema"]),
	}
}

func boundedSchema(decoded map[string]any, path string) []byte {
	data := payload.JSON(decoded, path)
	if len(data) > maxChoiceSchemaBytes {
		return nil
	}
	return data
}
