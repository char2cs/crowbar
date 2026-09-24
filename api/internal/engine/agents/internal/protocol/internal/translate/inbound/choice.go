package inbound

import (
	"strconv"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
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

func declaresChoice(fields spec.FieldMap) bool {
	for _, name := range choiceFields {
		if len(fields[name]) > 0 {
			return true
		}
	}
	return false
}

func permissionChoice(
	fields spec.FieldMap,
	decoded map[string]any,
) *models.ChoicePrompt {
	if !declaresChoice(fields) {
		return nil
	}
	promptID := mapping.String(decoded, fields["prompt_id"])
	toolName := mapping.String(decoded, fields["tool_name"])

	if questions := mapping.Objects(decoded, fields["questions"]); len(questions) > 0 {
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
	fields spec.FieldMap,
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
	fields spec.FieldMap,
	question map[string]any,
	index int,
) models.PromptQuestion {
	multi, _ := mapping.Bool(question, fields["question_multi"])
	id := "q" + strconv.Itoa(index)
	out := models.PromptQuestion{
		ID:    id,
		Title: mapping.String(question, fields["question_title"]),
		Text:  mapping.String(question, fields["question_text"]),
		Multi: multi,
	}
	for i, option := range mapping.Objects(question, fields["question_options"]) {
		if len(out.Options) >= maxChoiceOptions {
			break
		}
		out.Options = append(out.Options, models.ChoiceOption{
			ID:          id + "-answer-" + strconv.Itoa(i),
			Kind:        models.ChoiceOptionAnswer,
			Label:       mapping.String(option, fields["option_label"]),
			Description: mapping.String(option, fields["option_description"]),
		})
	}
	return out
}

func suggestionOptions(fields spec.FieldMap, decoded map[string]any) []models.ChoiceOption {
	suggestions := mapping.Objects(decoded, fields["suggestions"])
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
			Description: mapping.String(suggestion, fields["suggestion_description"]),
		})
	}
	return out
}

// suggestionLabel reads a LITERAL display string out of the descriptor, not a
// payload path: suggestion_label.* maps a provider's own machine name for a
// broader grant straight to English text the field map carries verbatim (see
// claude.yaml's own suggestion_label.* entries).
func suggestionLabel(fields spec.FieldMap, suggestion map[string]any) string {
	kind := mapping.String(suggestion, fields["suggestion_type"])
	if kind != "" {
		if label := literal(fields, suggestionLabelPrefix+kind); label != "" {
			return label
		}
	}
	return literal(fields, suggestionLabelDefault)
}

// literal reads a FieldMap entry as the single literal value it declares,
// never as a payload path — see suggestionLabel's own doc.
func literal(fields spec.FieldMap, key string) string {
	if v := fields[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

func elicitationChoice(
	fields spec.FieldMap,
	decoded map[string]any,
	message string,
) *models.ChoicePrompt {
	return &models.ChoicePrompt{
		Kind:     models.ChoiceElicitation,
		Title:    mapping.String(decoded, fields["mcp_server"]),
		Question: message,
		Mode:     mapping.String(decoded, fields["mode"]),
		Schema:   boundedSchema(decoded, fields["schema"]),
	}
}

func boundedSchema(decoded map[string]any, paths []string) []byte {
	data := mapping.JSON(decoded, paths)
	if len(data) > maxChoiceSchemaBytes {
		return nil
	}
	return data
}
