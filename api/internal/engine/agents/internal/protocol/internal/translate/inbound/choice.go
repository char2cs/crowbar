package inbound

import (
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
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

func permissionChoice(
	fields map[string]string,
	risk map[string][]string,
	decoded map[string]any,
) *models.ChoicePrompt {
	if !declaresChoice(fields) {
		return nil
	}
	promptID := firstNonEmpty(decoded, fields["prompt_id"])
	toolName := firstNonEmpty(decoded, fields["tool_name"])

	if questions := mapping.Objects(decoded, fields["questions"]); len(questions) > 0 {
		return questionChoice(fields, questions, promptID, toolName)
	}

	prompt := &models.ChoicePrompt{
		Kind:     models.ChoiceToolPermission,
		PromptID: promptID,
		ToolName: toolName,
		Risk:     classifyRisk(risk, toolName),
		Title:    toolName,

		Options: []models.ChoiceOption{
			{ID: models.ChoiceOptionAllow, Kind: models.ChoiceOptionAllow, Label: "Allow"},
			{ID: models.ChoiceOptionDeny, Kind: models.ChoiceOptionDeny, Label: "Deny"},
		},
	}
	prompt.Options = append(prompt.Options, suggestionOptions(fields, decoded)...)
	return prompt
}

// classifyRisk maps toolName to the tier the descriptor's own risk: table
// declares for it. Unmatched — including every name when the descriptor
// declares no risk: block — is models.RiskSensitive: the table is a safe
// allowlist, never a denylist.
func classifyRisk(risk map[string][]string, toolName string) models.RiskTier {
	switch {
	case matchesAny(risk[string(models.RiskInternal)], toolName):
		return models.RiskInternal
	case matchesAny(risk[string(models.RiskReadOnly)], toolName):
		return models.RiskReadOnly
	case matchesAny(risk[string(models.RiskStandard)], toolName):
		return models.RiskStandard
	default:
		return models.RiskSensitive
	}
}

func matchesAny(patterns []string, toolName string) bool {
	for _, pattern := range patterns {
		if matchesPattern(pattern, toolName) {
			return true
		}
	}
	return false
}

// matchesPattern supports one wildcard shape — a trailing "*" — which is all
// Crowbar's own MCP tool names need (they share one "mcp__crowbar__" prefix).
// Anything else must match the provider's tool name exactly.
func matchesPattern(pattern, toolName string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(toolName, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == toolName
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
	multi, _ := mapping.Bool(question, fields["question_multi"])
	id := "q" + strconv.Itoa(index)
	out := models.PromptQuestion{
		ID:    id,
		Title: firstNonEmpty(question, fields["question_title"]),
		Text:  firstNonEmpty(question, fields["question_text"]),
		Multi: multi,
	}
	for i, option := range mapping.Objects(question, fields["question_options"]) {
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
	data := mapping.JSON(decoded, path)
	if len(data) > maxChoiceSchemaBytes {
		return nil
	}
	return data
}
