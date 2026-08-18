package hooks

import (
	"strconv"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
)

// maxChoiceOptions bounds how many options one prompt may carry.
//
// A prompt is open state — it lives in the aggregate until it is answered — so an
// unbounded option list would be an unbounded aggregate. The cap is far above any
// real prompt: the AskUserQuestion payloads measured against claude 2.1.234 on
// 2026-08-17 carried two.
const maxChoiceOptions = 32

// maxChoiceSchemaBytes bounds a requested-input schema for the same reason.
const maxChoiceSchemaBytes = 8 << 10

// choiceFields is the choice vocabulary a descriptor must name before a
// permission reports a prompt at all.
//
// It is what makes this addition free for a descriptor that does not take it: the
// two shipped descriptors used to map `permission: {session_id, message}` and
// nothing else, and a descriptor still mapping only that pair keeps EXACTLY its
// previous behaviour — an interruption, and no prompt.
var choiceFields = [...]string{"prompt_id", "tool_name", "tool_input", "questions", "suggestions"}

func declaresChoice(fields map[string]string) bool {
	for _, name := range choiceFields {
		if fields[name] != "" {
			return true
		}
	}
	return false
}

// permissionChoice builds the prompt a permission request is asking.
//
// Two different prompts arrive down this one event. Most are "may I run this
// tool", answered allow or deny. But AskUserQuestion is a TOOL on claude rather
// than an event of its own (measured against 2.1.234 on 2026-08-17), and its
// permission carries the question and its labelled options in the very same tool
// input — so the shape of the payload, not the name of the hook, is what says
// which of the two this is.
func permissionChoice(fields map[string]string, decoded map[string]any) *models.ChoicePrompt {
	if !declaresChoice(fields) {
		return nil
	}
	promptID := firstNonEmpty(decoded, fields["prompt_id"])
	toolName := firstNonEmpty(decoded, fields["tool_name"])

	if questions := payload.Objects(decoded, fields["questions"]); len(questions) > 0 {
		return questionChoice(fields, questions[0], promptID, toolName)
	}

	prompt := &models.ChoicePrompt{
		Kind:     models.ChoiceToolPermission,
		PromptID: promptID,
		ToolName: toolName,
		Title:    toolName,
		// Allow and deny are not read out of the payload because no provider
		// enumerates them: they are what a permission prompt IS. The labels are
		// Crowbar's own words, which is why a client should render from Kind.
		Options: []models.ChoiceOption{
			{ID: models.ChoiceOptionAllow, Kind: models.ChoiceOptionAllow, Label: "Allow"},
			{ID: models.ChoiceOptionDeny, Kind: models.ChoiceOptionDeny, Label: "Deny"},
		},
	}
	prompt.Options = append(prompt.Options, suggestionOptions(fields, decoded)...)
	return prompt
}

// questionChoice reads the FIRST question of a payload that may carry several.
//
// Claude's AskUserQuestion input is an array, and only its first entry is
// modelled: a Crowbar choice is one question with one set of options, and a second
// question is a second pending record rather than a second shape. Every payload
// measured against 2.1.234 on 2026-08-17 carried exactly one.
func questionChoice(
	fields map[string]string,
	question map[string]any,
	promptID, toolName string,
) *models.ChoicePrompt {
	multi, _ := payload.Bool(question, fields["question_multi"])
	prompt := &models.ChoicePrompt{
		Kind:     models.ChoiceQuestion,
		PromptID: promptID,
		ToolName: toolName,
		Title:    firstNonEmpty(question, fields["question_title"]),
		Question: firstNonEmpty(question, fields["question_text"]),
		Multi:    multi,
	}
	for i, option := range payload.Objects(question, fields["question_options"]) {
		if len(prompt.Options) >= maxChoiceOptions {
			break
		}
		prompt.Options = append(prompt.Options, models.ChoiceOption{
			ID:          "answer-" + strconv.Itoa(i),
			Kind:        models.ChoiceOptionAnswer,
			Label:       firstNonEmpty(option, fields["option_label"]),
			Description: firstNonEmpty(option, fields["option_description"]),
		})
	}
	return prompt
}

// suggestionOptions reads the broader grants a provider proposes alongside the
// plain allow — claude's permission_suggestions, "and stop asking about this
// directory".
//
// A suggestion with no label is skipped rather than rendered blank: an unnamed
// button that changes a permission mode is worse than no button.
func suggestionOptions(fields map[string]string, decoded map[string]any) []models.ChoiceOption {
	suggestions := payload.Objects(decoded, fields["suggestions"])
	out := make([]models.ChoiceOption, 0, len(suggestions))
	for i, suggestion := range suggestions {
		if len(out) >= maxChoiceOptions {
			break
		}
		label := firstNonEmpty(suggestion, fields["suggestion_label"])
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

// elicitationChoice builds the prompt an MCP server is asking through the CLI.
//
// It carries no options, and that is the honest answer rather than a gap: what an
// elicitation offers is a SCHEMA, and a client renders the form from it exactly as
// any other MCP client would. Inventing options by reading the schema here would
// put JSON-Schema interpretation inside a package whose whole job is to not
// interpret anything.
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

// boundedSchema DROPS an oversized schema rather than truncating it: half a JSON
// document is not a smaller schema, it is an unparseable one, and a client that
// cannot parse the form is better off knowing there is none.
func boundedSchema(decoded map[string]any, path string) []byte {
	data := payload.JSON(decoded, path)
	if len(data) > maxChoiceSchemaBytes {
		return nil
	}
	return data
}
