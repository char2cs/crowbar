package hooks_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/hooks"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// permissionMap mirrors the shipped claude descriptor's permission block. It is
// spelled out rather than loaded so this package keeps testing the MAPPING
// machinery; that the descriptor on disk actually carries these paths is proved
// end-to-end where a real claude chat is driven.
func permissionMap() map[string]string {
	return map[string]string{
		"session_id":             "session_id",
		"message":                "tool_name",
		"prompt_id":              "prompt_id",
		"tool_name":              "tool_name",
		"tool_input":             "tool_input",
		"suggestions":            "permission_suggestions",
		"suggestion_label":       "type",
		"suggestion_description": "mode,destination",
		"questions":              "tool_input.questions",
		"question_title":         "header",
		"question_text":          "question",
		"question_options":       "options",
		"question_multi":         "multiSelect",
		"option_label":           "label",
		"option_description":     "description",
	}
}

// The payload captured live from claude 2.1.234 on 2026-08-17. Everything but the
// tool name used to be discarded.
const permissionPayload = `{
  "session_id":"s1","prompt_id":"81899da5","permission_mode":"default",
  "hook_event_name":"PermissionRequest","tool_name":"Bash",
  "tool_input":{"command":"touch PROOF","description":"Create proof control file"},
  "permission_suggestions":[
    {"type":"addDirectories","directories":["/proof"],"destination":"session"},
    {"type":"setMode","mode":"acceptEdits","destination":"session"}]}`

func TestParse_PermissionCarriesTheWholePrompt(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})

	ev, err := hooks.Parse(d, spec.HookPermission, []byte(permissionPayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Interrupt, "a permission is still an interruption")
	assert.Equal(t, models.InterruptPermission, ev.Interrupt.Kind)

	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceToolPermission, ev.Choice.Kind)
	assert.Equal(t, "81899da5", ev.Choice.PromptID)
	assert.Equal(t, "Bash", ev.Choice.ToolName)
	assert.Equal(t, "Bash", ev.Choice.Title)

	require.Len(t, ev.Choice.Options, 4, "allow, deny, and both suggestions")
	assert.Equal(t, models.ChoiceOptionAllow, ev.Choice.Options[0].Kind)
	assert.Equal(t, models.ChoiceOptionDeny, ev.Choice.Options[1].Kind)
	assert.Equal(t, models.ChoiceOptionSuggestion, ev.Choice.Options[2].Kind)
	assert.Equal(t, "addDirectories", ev.Choice.Options[2].Label)
	assert.Equal(t, "session", ev.Choice.Options[2].Description,
		"the alternation falls through to destination when the suggestion has no mode")
	assert.Equal(t, "setMode", ev.Choice.Options[3].Label)
	assert.Equal(t, "acceptEdits", ev.Choice.Options[3].Description)
}

// The permission carries no tool_use_id — claude's own documentation claims one
// and the payload does not have it — so nothing downstream may expect the engine
// to supply the identity of the call being gated.
func TestParse_PermissionOffersNoToolCallID(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookPermission: mergeMap(permissionMap(), map[string]string{"tool_id": "tool_use_id"}),
	})

	ev, err := hooks.Parse(d, spec.HookPermission, []byte(permissionPayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Nil(t, ev.Tool, "a permission is not a tool invocation")
}

// AskUserQuestion is a TOOL, so its question arrives inside the permission's own
// tool input rather than as an event of its own.
func TestParse_AskUserQuestionBecomesAQuestionChoice(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	raw := []byte(`{"session_id":"s1","prompt_id":"p9","tool_name":"AskUserQuestion",
	  "tool_input":{"questions":[{"question":"Do you prefer option A or option B?",
	    "header":"Pick","options":[{"label":"A","description":"Option A"},
	    {"label":"B","description":"Option B"}],"multiSelect":false}]}}`)

	ev, err := hooks.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceQuestion, ev.Choice.Kind)
	assert.Equal(t, "Pick", ev.Choice.Title)
	assert.Equal(t, "Do you prefer option A or option B?", ev.Choice.Question)
	assert.False(t, ev.Choice.Multi)
	require.Len(t, ev.Choice.Options, 2)
	assert.Equal(t, models.ChoiceOptionAnswer, ev.Choice.Options[0].Kind)
	assert.Equal(t, "A", ev.Choice.Options[0].Label)
	assert.Equal(t, "Option A", ev.Choice.Options[0].Description)
	assert.NotEqual(t, ev.Choice.Options[0].ID, ev.Choice.Options[1].ID,
		"an answer must be able to name one option without echoing its label")
}

func TestParse_AMultiSelectQuestionSaysSo(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	raw := []byte(`{"tool_name":"AskUserQuestion","tool_input":{"questions":[
	  {"question":"which?","options":[{"label":"A"}],"multiSelect":true}]}}`)

	ev, err := hooks.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.True(t, ev.Choice.Multi)
}

// A suggestion with no label would render as an unnamed button that changes a
// permission mode, which is worse than no button at all.
func TestParse_AnUnlabelledSuggestionIsSkipped(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	raw := []byte(`{"tool_name":"Bash","permission_suggestions":[{"destination":"session"}]}`)

	ev, err := hooks.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Len(t, ev.Choice.Options, 2, "allow and deny, and nothing nameless beside them")
}

func TestParse_ElicitationCarriesTheServerModeAndSchema(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookElicitation: {
			"session_id": "session_id", "message": "message",
			"mcp_server": "mcp_server_name", "mode": "mode", "schema": "requested_schema",
		},
	})
	raw := []byte(`{"hook_event_name":"Elicitation","mcp_server_name":"spike",
	  "message":"do you prefer A or B?","mode":"form",
	  "requested_schema":{"type":"object","properties":{"choice":{"type":"string",
	  "enum":["A","B"]}},"required":["choice"]}}`)

	ev, err := hooks.Parse(d, spec.HookElicitation, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Interrupt)
	assert.Equal(t, models.InterruptElicitation, ev.Interrupt.Kind,
		"the elicitation interruption kind must actually be produced")
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceElicitation, ev.Choice.Kind)
	assert.Equal(t, "spike", ev.Choice.Title)
	assert.Equal(t, "do you prefer A or B?", ev.Choice.Question)
	assert.Equal(t, "form", ev.Choice.Mode)
	assert.Contains(t, string(ev.Choice.Schema), `"enum":["A","B"]`)
	assert.Empty(t, ev.Choice.Options,
		"an elicitation offers a schema, and inventing buttons from it would be a guess")
}

// Half a JSON document is not a smaller schema, it is an unparseable one.
func TestParse_AnOversizedSchemaIsDroppedNotTruncated(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookElicitation: {"message": "message", "schema": "requested_schema"},
	})
	raw := []byte(`{"message":"pick","requested_schema":{"blob":"` +
		strings.Repeat("x", 9<<10) + `"}}`)

	ev, err := hooks.Parse(d, spec.HookElicitation, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Nil(t, ev.Choice.Schema)
	assert.Equal(t, "pick", ev.Choice.Question, "the question survives the schema being dropped")
}

func TestParse_ToolFailCarriesTheErrorAndDuration(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookToolFail: {
			"tool_id": "tool_use_id", "tool_name": "tool_name",
			"tool_result": "tool_response,error", "tool_error": "error",
			"duration_ms": "duration_ms",
		},
	})
	raw := []byte(`{"tool_use_id":"t1","tool_name":"Bash","error":"exit status 1",
	  "is_interrupt":false,"duration_ms":42}`)

	ev, err := hooks.Parse(d, spec.HookToolFail, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "t1", ev.Tool.ID)
	assert.Equal(t, "exit status 1", ev.Tool.Error)
	assert.Equal(t, 42, ev.Tool.DurationMS)
	assert.Equal(t, "exit status 1", string(ev.Tool.Result),
		"the result alternation falls through to error when there is no response")
}

func TestParse_ToolResultAlternationPrefersTheResponse(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookToolFail: {"tool_result": "tool_response,error", "tool_error": "error"},
	})

	ev, err := hooks.Parse(d, spec.HookToolFail,
		[]byte(`{"tool_response":"partial output","error":"exit status 1"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "partial output", string(ev.Tool.Result))
}

// The degradation guarantee, stated as a test: a descriptor that maps none of the
// new vocabulary reports nothing new and behaves exactly as it did.
func TestParse_ADescriptorMappingNoChoiceVocabularyReportsNoPrompt(t *testing.T) {
	// This IS the mapping both descriptors shipped before prompts existed.
	d := descriptor(map[string]map[string]string{
		spec.HookPermission: {"session_id": "session_id", "message": "tool_name"},
	})

	ev, err := hooks.Parse(d, spec.HookPermission, []byte(permissionPayload))

	require.NoError(t, err)
	assert.Nil(t, ev.Choice, "no choice vocabulary declared, so no prompt is reported")
	require.NotNil(t, ev.Interrupt)
	assert.Equal(t, models.InterruptPermission, ev.Interrupt.Kind)
	assert.Equal(t, "Bash", ev.Interrupt.Detail, "and the previous behaviour is untouched")
}

func TestParse_AnUndeclaredNewKindNeverFires(t *testing.T) {
	testCases := []struct {
		name      string
		canonical string
	}{
		{name: "elicitation", canonical: spec.HookElicitation},
		{name: "tool failure", canonical: spec.HookToolFail},
	}
	// A descriptor for a provider that has neither concept — which is codex today.
	d := descriptor(map[string]map[string]string{
		spec.HookPermission: {"session_id": "session_id", "message": "tool_name"},
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := hooks.Parse(d, tc.canonical, []byte(`{"message":"anything"}`))

			assert.ErrorIs(t, err, hooks.ErrUndeclaredEvent,
				"an unmapped kind must degrade to never being reported")
		})
	}
}

// Declared is what a client reads to know what an agent can and cannot observe, so
// the new kinds have to appear in it or a UI would hide a capability that exists.
func TestDeclared_IncludesTheNewKinds(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookElicitation: {"message": "message"},
		spec.HookToolFail:    {"tool_id": "tool_use_id"},
	})

	assert.Equal(t, []string{spec.HookElicitation, spec.HookToolFail}, hooks.Declared(d))
}

func mergeMap(base, extra map[string]string) map[string]string {
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// A prompt is OPEN STATE — it lives in the aggregate until it is answered — so an
// unbounded option list would be an unbounded aggregate.
func TestParse_AnAbsurdOptionListIsCapped(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	options := make([]string, 0, 100)
	suggestions := make([]string, 0, 100)
	for i := range 100 {
		options = append(options, `{"label":"opt`+strconv.Itoa(i)+`"}`)
		suggestions = append(suggestions, `{"type":"sug`+strconv.Itoa(i)+`"}`)
	}

	question, err := hooks.Parse(d, spec.HookPermission, []byte(
		`{"tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"q",
		 "options":[`+strings.Join(options, ",")+`]}]}}`))
	require.NoError(t, err)
	require.NotNil(t, question.Choice)
	assert.Len(t, question.Choice.Options, 32)

	permission, err := hooks.Parse(d, spec.HookPermission, []byte(
		`{"tool_name":"Bash","permission_suggestions":[`+strings.Join(suggestions, ",")+`]}`))
	require.NoError(t, err)
	require.NotNil(t, permission.Choice)
	assert.Len(t, permission.Choice.Options, 34, "allow and deny, then the capped suggestions")
}
