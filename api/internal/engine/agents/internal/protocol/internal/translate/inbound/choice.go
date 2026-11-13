package inbound

import (
	"strconv"
	"strings"

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

// markReasons flags the options whose declared reply carries the person's reason, so a
// card offers a reason only where the agent will receive it.
func markReasons(options []models.ChoiceOption, d *spec.Descriptor, canonical string) {
	answer, ok := d.AnswerFor(canonical)
	if !ok {
		return
	}
	for i := range options {
		options[i].Reason = strings.Contains(answer.Responses[options[i].ID], "{reason_json}")
	}
}

func permissionChoice(
	fields spec.FieldMap,
	decoded map[string]any,
) *models.ChoicePrompt {
	if !declaresChoice(fields) {
		return nil
	}
	prompt := toolChoice(fields, decoded)
	prompt.ToolTarget = mapping.String(decoded, fields["tool_target"])
	prompt.ToolInput = mapping.JSON(decoded, fields["tool_input"])
	prompt.MCPServer = mapping.String(decoded, fields["mcp_server"])
	return prompt
}

func toolChoice(fields spec.FieldMap, decoded map[string]any) *models.ChoicePrompt {
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
	id := mapping.String(question, fields["question_id"])
	if id == "" {
		id = "q" + strconv.Itoa(index)
	}
	out := models.PromptQuestion{
		ID:    id,
		Title: mapping.String(question, fields["question_title"]),
		Text:  mapping.String(question, fields["question_text"]),
		Multi: multi,
	}
	// Options are objects (question_options) or bare strings (question_choices).
	options := make([]models.ChoiceOption, 0)
	for _, option := range mapping.Objects(question, fields["question_options"]) {
		options = append(options, models.ChoiceOption{
			Label:       mapping.String(option, fields["option_label"]),
			Description: mapping.String(option, fields["option_description"]),
		})
	}
	for _, label := range mapping.Strings(question, fields["question_choices"]) {
		options = append(options, models.ChoiceOption{Label: label})
	}
	for i, option := range options[:min(len(options), maxChoiceOptions)] {
		option.ID, option.Kind = id+"-answer-"+strconv.Itoa(i), models.ChoiceOptionAnswer
		out.Options = append(out.Options, option)
	}
	return out
}

func suggestionOptions(fields spec.FieldMap, suggestions []any) []models.ChoiceOption {
	out := make([]models.ChoiceOption, 0, len(suggestions))
	for i, suggestion := range suggestions {
		if len(out) >= maxChoiceOptions {
			break
		}
		object, _ := suggestion.(map[string]any)
		description := mapping.String(object, fields["suggestion_description"])
		label, exact := suggestionLabel(fields, suggestion, description)
		if label == "" {
			continue
		}
		if exact {
			description = ""
		}
		out = append(out, models.ChoiceOption{
			ID:          models.SuggestionOptionID(i),
			Kind:        models.ChoiceOptionSuggestion,
			Label:       label,
			Description: description,
		})
	}
	return out
}

// suggestionLabel reads a LITERAL display string out of the descriptor, not a
// payload path: suggestion_label.* maps a provider's machine name for a broader
// grant to the English text the field map carries verbatim. exact when a label
// names the grant's description too (suggestion_label.<type>.<description>). An event
// that declares no suggestion_type names each offer by itself (spec.OfferName).
func suggestionLabel(fields spec.FieldMap, suggestion any, description string) (string, bool) {
	kind := spec.OfferName(suggestion)
	if typed := fields["suggestion_type"]; len(typed) > 0 {
		object, _ := suggestion.(map[string]any)
		kind = mapping.String(object, typed)
	}
	if kind == "" {
		return literal(fields, suggestionLabelDefault), false
	}
	if label := literal(fields, suggestionLabelPrefix+kind+"."+description); description != "" && label != "" {
		return label, true
	}
	if label := literal(fields, suggestionLabelPrefix+kind); label != "" {
		return label, false
	}
	return literal(fields, suggestionLabelDefault), false
}

// literal reads a FieldMap entry as the single literal value it declares,
// never as a payload path — see suggestionLabel's own doc.
func literal(fields spec.FieldMap, key string) string {
	return fields[key].Literal()
}

func elicitationChoice(
	fields spec.FieldMap,
	decoded map[string]any,
	message string,
) *models.ChoicePrompt {
	return &models.ChoicePrompt{
		Kind:      models.ChoiceElicitation,
		Title:     mapping.String(decoded, fields["mcp_server"]),
		MCPServer: mapping.String(decoded, fields["mcp_server"]),
		Question:  message,
		Mode:      mapping.String(decoded, fields["mode"]),
		Schema:    boundedSchema(decoded, fields["schema"]),
	}
}

func boundedSchema(decoded map[string]any, paths mapping.Expr) []byte {
	data := mapping.JSON(decoded, paths)
	if len(data) > maxChoiceSchemaBytes {
		return nil
	}
	return data
}
