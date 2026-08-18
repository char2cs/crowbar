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

// maxChoiceQuestions bounds how many QUESTIONS one prompt may carry, for the same
// aggregate-size reason.
//
// What happens past it is deliberately not "keep the first 32". Dropping a
// question is precisely the defect this list exists to fix: an answer that covers
// some of what was asked hands the CLI a partial `updatedInput`, and the agent
// goes on waiting for answers nothing can send. So a payload over the bound is
// modelled with NO questions at all — the prompt is still recorded and still
// legible, it simply offers nothing to press, which lands it on the same
// read-only card as a prompt whose relay has already timed out. That is safe
// because the CLI's own picker is up the whole time the hook is held: the human
// answers it there, exactly as they would on a machine with no Crowbar installed.
const maxChoiceQuestions = 32

// maxChoiceSchemaBytes bounds a requested-input schema for the same reason.
const maxChoiceSchemaBytes = 8 << 10

// suggestionLabelPrefix and suggestionLabelDefault are how a descriptor declares
// HUMAN text for the broader grants a provider proposes.
//
// The provider names its own suggestions in its own vocabulary — claude's
// permission_suggestions carry `type: "addRules"` — and putting that string on a
// control is how "addRules" ended up looking like a real choice a user could
// press. The mapping from that machine name to English is PROVIDER VOCABULARY, so
// it is declared in the descriptor and never written in Go; what lives here is
// only the shape of the declaration.
const (
	suggestionLabelPrefix  = "suggestion_label."
	suggestionLabelDefault = suggestionLabelPrefix + "default"
)

// choiceFields is the choice vocabulary a descriptor must name before a
// permission reports a prompt at all.
//
// It is what makes this addition free for a descriptor that does not take it: the
// two shipped descriptors used to map `permission: {session_id, message}` and
// nothing else, and a descriptor still mapping only that pair keeps EXACTLY its
// previous behaviour — an interruption, and no prompt.
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
		return questionChoice(fields, questions, promptID, toolName)
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

// questionChoice models EVERY question a payload carries.
//
// Claude's AskUserQuestion input is an array and a user can ask for three
// questions at once, so all of them are modelled. Only modelling the first is a
// measured user-blocking defect: Crowbar recorded one question, the human answered
// it, the CLI was handed an `updatedInput` covering one of three, and it went on
// saying "still waiting on your answers to questions 2 & 3" — unanswerable from
// the chat, forever.
//
// The questions list is the ONLY representation of a question prompt. Nothing is
// copied up to prompt-level Options, so there is exactly one place to read an
// answerable question's options from, and one code path whether the payload
// carried one question or ten.
//
// Prompt-level Title and Question are the CARD's headline and are filled only
// when there is a single question to be the headline of. With several, no one
// question is what the prompt is asking, and naming one of them would be a lie a
// reader could act on.
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
		// Modelled with no questions rather than with the first 32 — see
		// maxChoiceQuestions. A partial model is the very defect this list fixes.
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

// choiceQuestion models one entry of the questions array.
//
// Option ids are namespaced by the question's own index because an answer names
// its picks in ONE FLAT LIST: "q0-answer-1" says which question it answers, where
// a bare "answer-1" from three questions would say nothing at all.
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

// suggestionOptions reads the broader grants a provider proposes alongside the
// plain allow — claude's permission_suggestions, "and stop asking about this
// directory".
//
// The label is NEVER the provider's own machine name. Reading `type` straight
// onto the option put "addRules" on screen as though it were a choice a person
// could make, spelled in a vocabulary only the CLI's source uses — and sitting on
// the one path the backend refuses with a 400, because no response template for a
// suggestion has ever been measured. So the type is looked up in the descriptor's
// declared vocabulary, and an unrecognised one takes the declared generic text.
//
// A suggestion the descriptor can put no words to at all is SKIPPED rather than
// rendered blank or rendered raw: an unnamed thing that changes a permission mode
// is worse than nothing on the screen.
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

// suggestionLabel translates one suggestion's provider type into the human words
// the descriptor declares for it.
//
// Both halves of the lookup are the descriptor's: the PATH to the provider's type
// value, and the text each captured value gets. Nothing here knows what any of
// those values mean, which is what keeps a provider's vocabulary out of Go.
func suggestionLabel(fields map[string]string, suggestion map[string]any) string {
	kind := firstNonEmpty(suggestion, fields["suggestion_type"])
	if kind != "" {
		if label := fields[suggestionLabelPrefix+kind]; label != "" {
			return label
		}
	}
	// A type nobody has captured still gets said — in general terms, and honestly.
	// Silence would hide that the provider offered something, and the raw value is
	// what this function exists to keep off the screen.
	return fields[suggestionLabelDefault]
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
