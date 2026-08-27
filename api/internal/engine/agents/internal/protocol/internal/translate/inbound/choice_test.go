package inbound_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func permissionMap() map[string]string {
	return map[string]string{
		"session_id":             "session_id",
		"message":                "tool_name",
		"prompt_id":              "prompt_id",
		"tool_name":              "tool_name",
		"tool_input":             "tool_input",
		"suggestions":            "permission_suggestions",
		"suggestion_type":        "type",
		"suggestion_description": "mode,destination",

		"suggestion_label.addRules":       "Add a permanent rule for this",
		"suggestion_label.addDirectories": "Allow this directory from now on",
		"suggestion_label.setMode":        "Switch to a more permissive mode",
		"suggestion_label.default":        "A broader permission than this one",
		"questions":                       "tool_input.questions",
		"question_title":                  "header",
		"question_text":                   "question",
		"question_options":                "options",
		"question_multi":                  "multiSelect",
		"option_label":                    "label",
		"option_description":              "description",
	}
}

const permissionPayload = `{
  "session_id":"s1","prompt_id":"81899da5","permission_mode":"default",
  "hook_event_name":"PermissionRequest","tool_name":"Bash",
  "tool_input":{"command":"touch PROOF","description":"Create proof control file"},
  "permission_suggestions":[
    {"type":"addDirectories","directories":["/proof"],"destination":"session"},
    {"type":"setMode","mode":"acceptEdits","destination":"session"}]}`

const threeQuestionPayload = `{
  "session_id":"s1","prompt_id":"p3","hook_event_name":"PermissionRequest",
  "tool_name":"AskUserQuestion",
  "tool_input":{"questions":[
    {"question":"Which language?","header":"Language","multiSelect":false,
     "options":[{"label":"Go","description":"the daemon"},{"label":"TypeScript"}]},
    {"question":"Which databases?","header":"Storage","multiSelect":true,
     "options":[{"label":"SQLite"},{"label":"Postgres"},{"label":"Redis"}]},
    {"question":"Deploy where?","header":"Target","multiSelect":false,
     "options":[{"label":"Local"},{"label":"Cloud"}]}]}}`

func TestParse_PermissionCarriesTheWholePrompt(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(permissionPayload))

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
	assert.Equal(t, "Allow this directory from now on", ev.Choice.Options[2].Label)
	assert.Equal(t, "session", ev.Choice.Options[2].Description,
		"the alternation falls through to destination when the suggestion has no mode")
	assert.Equal(t, "Switch to a more permissive mode", ev.Choice.Options[3].Label)
	assert.Equal(t, "acceptEdits", ev.Choice.Options[3].Description)
	assert.Empty(t, ev.Choice.Questions, "a permission asks nothing beyond may-I")
}

func TestRegression_ASuggestionIsNeverLabelledWithARawProviderTypeName(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})

	raw := []byte(`{"tool_name":"Bash","permission_suggestions":[
	  {"type":"addRules","destination":"session"},
	  {"type":"addDirectories","destination":"session"},
	  {"type":"setMode","mode":"acceptEdits"},
	  {"type":"someTypeNobodyHasSeen","destination":"session"}]}`)

	ev, err := inbound.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	require.Len(t, ev.Choice.Options, 6, "allow, deny, and all four suggestions")
	for _, option := range ev.Choice.Options {
		for _, machineName := range []string{
			"addRules", "addDirectories", "setMode", "someTypeNobodyHasSeen",
		} {
			assert.NotEqual(t, machineName, option.Label,
				"a provider's own type value must never reach a label")
			assert.NotContains(t, option.Label, machineName)
		}
	}
	assert.Equal(t, "Add a permanent rule for this", ev.Choice.Options[2].Label)
	assert.Equal(t, "A broader permission than this one", ev.Choice.Options[5].Label,
		"a type nobody has captured takes the declared generic text, not its own name")
}

func TestParse_ASuggestionWithNoDeclaredWordsIsSkipped(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookPermission: {
			"tool_name": "tool_name", "suggestions": "permission_suggestions",
			"suggestion_type": "type",
		},
	})

	ev, err := inbound.Parse(d, spec.HookPermission,
		[]byte(`{"tool_name":"Bash","permission_suggestions":[{"type":"addRules"}]}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Len(t, ev.Choice.Options, 2, "allow and deny, and nothing nameless beside them")
}

func TestParse_PermissionOffersNoToolCallID(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookPermission: mergeMap(permissionMap(), map[string]string{"tool_id": "tool_use_id"}),
	})

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(permissionPayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Nil(t, ev.Tool, "a permission is not a tool invocation")
}

func TestParse_AskUserQuestionBecomesAQuestionChoice(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	raw := []byte(`{"session_id":"s1","prompt_id":"p9","tool_name":"AskUserQuestion",
	  "tool_input":{"questions":[{"question":"Do you prefer option A or option B?",
	    "header":"Pick","options":[{"label":"A","description":"Option A"},
	    {"label":"B","description":"Option B"}],"multiSelect":false}]}}`)

	ev, err := inbound.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceQuestion, ev.Choice.Kind)
	assert.Equal(t, "Pick", ev.Choice.Title)
	assert.Equal(t, "Do you prefer option A or option B?", ev.Choice.Question)
	assert.Empty(t, ev.Choice.Options,
		"a question's options live on the question, so there is exactly one place to read them")

	require.Len(t, ev.Choice.Questions, 1)
	question := ev.Choice.Questions[0]
	assert.Equal(t, "Pick", question.Title)
	assert.Equal(t, "Do you prefer option A or option B?", question.Text)
	assert.False(t, question.Multi)
	require.Len(t, question.Options, 2)
	assert.Equal(t, models.ChoiceOptionAnswer, question.Options[0].Kind)
	assert.Equal(t, "A", question.Options[0].Label)
	assert.Equal(t, "Option A", question.Options[0].Description)
	assert.NotEqual(t, question.Options[0].ID, question.Options[1].ID,
		"an answer must be able to name one option without echoing its label")
}

func TestRegression_EveryQuestionOfAMultiQuestionPayloadIsModelled(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(threeQuestionPayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceQuestion, ev.Choice.Kind)
	require.Len(t, ev.Choice.Questions, 3, "three questions asked is three questions modelled")

	assert.Equal(t, "Which language?", ev.Choice.Questions[0].Text)
	assert.False(t, ev.Choice.Questions[0].Multi)
	assert.Equal(t, "Which databases?", ev.Choice.Questions[1].Text)
	assert.True(t, ev.Choice.Questions[1].Multi,
		"multiSelect rides each question, so one prompt can mix the two shapes")
	assert.Equal(t, "Deploy where?", ev.Choice.Questions[2].Text)

	assert.Empty(t, ev.Choice.Question)
	assert.Empty(t, ev.Choice.Title)

	seen := map[string]bool{}
	for _, q := range ev.Choice.Questions {
		require.NotEmpty(t, q.Options)
		for _, option := range q.Options {
			assert.False(t, seen[option.ID], "option id %q is not unique across the prompt", option.ID)
			seen[option.ID] = true
		}
	}
	assert.Len(t, seen, 7)
}

func TestParse_AMultiSelectQuestionSaysSo(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	raw := []byte(`{"tool_name":"AskUserQuestion","tool_input":{"questions":[
	  {"question":"which?","options":[{"label":"A"}],"multiSelect":true}]}}`)

	ev, err := inbound.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	require.Len(t, ev.Choice.Questions, 1)
	assert.True(t, ev.Choice.Questions[0].Multi)
}

func TestParse_AnAbsurdQuestionListIsModelledWithNoQuestionsAtAll(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	questions := make([]string, 0, 40)
	for i := range 40 {
		questions = append(questions,
			`{"question":"q`+strconv.Itoa(i)+`","options":[{"label":"yes"}]}`)
	}

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(
		`{"tool_name":"AskUserQuestion","tool_input":{"questions":[`+
			strings.Join(questions, ",")+`]}}`,
	))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceQuestion, ev.Choice.Kind, "it is still recorded as a question")
	assert.Empty(t, ev.Choice.Questions,
		"a PARTIAL model is the defect; none at all sends the human to the terminal")
	assert.Empty(t, ev.Choice.Options)
}

func TestParse_AnUntypedSuggestionTakesTheDeclaredGenericWords(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	raw := []byte(`{"tool_name":"Bash","permission_suggestions":[{"destination":"session"}]}`)

	ev, err := inbound.Parse(d, spec.HookPermission, raw)

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	require.Len(t, ev.Choice.Options, 3)
	assert.Equal(t, "A broader permission than this one", ev.Choice.Options[2].Label)
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

	ev, err := inbound.Parse(d, spec.HookElicitation, raw)

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

func TestParse_AnOversizedSchemaIsDroppedNotTruncated(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookElicitation: {"message": "message", "schema": "requested_schema"},
	})
	raw := []byte(`{"message":"pick","requested_schema":{"blob":"` +
		strings.Repeat("x", 9<<10) + `"}}`)

	ev, err := inbound.Parse(d, spec.HookElicitation, raw)

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

	ev, err := inbound.Parse(d, spec.HookToolFail, raw)

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

	ev, err := inbound.Parse(d, spec.HookToolFail,
		[]byte(`{"tool_response":"partial output","error":"exit status 1"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Tool)
	assert.Equal(t, "partial output", string(ev.Tool.Result))
}

func TestParse_ADescriptorMappingNoChoiceVocabularyReportsNoPrompt(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookPermission: {"session_id": "session_id", "message": "tool_name"},
	})

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(permissionPayload))

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

	d := descriptor(map[string]map[string]string{
		spec.HookPermission: {"session_id": "session_id", "message": "tool_name"},
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := inbound.Parse(d, tc.canonical, []byte(`{"message":"anything"}`))

			assert.ErrorIs(t, err, inbound.ErrUndeclaredEvent,
				"an unmapped kind must degrade to never being reported")
		})
	}
}

func TestDeclared_IncludesTheNewKinds(t *testing.T) {
	d := descriptor(map[string]map[string]string{
		spec.HookElicitation: {"message": "message"},
		spec.HookToolFail:    {"tool_id": "tool_use_id"},
	})

	assert.Equal(t, []string{spec.HookElicitation, spec.HookToolFail}, inbound.Declared(d))
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

func TestParse_PermissionClassifiesRiskFromTheDescriptorsTable(t *testing.T) {
	risk := map[string][]string{
		"read-only": {"Read", "Grep"},
		"standard":  {"Bash", "Edit"},
		"internal":  {"mcp__crowbar__*"},
	}
	d := descriptorWithRisk(permissionMap(), risk)

	cases := []struct {
		name     string
		toolName string
		want     models.RiskTier
	}{
		{"read-only tool", "Read", models.RiskReadOnly},
		{"standard tool", "Bash", models.RiskStandard},
		{"crowbar's own mcp tool matches the wildcard", "mcp__crowbar__post_review_comment", models.RiskInternal},
		{"unclassified tool defaults to sensitive", "WebFetch", models.RiskSensitive},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"session_id":"s1","prompt_id":"p1","tool_name":"` + tc.toolName + `"}`)
			ev, err := inbound.Parse(d, spec.HookPermission, payload)
			require.NoError(t, err)
			require.NotNil(t, ev.Choice)
			assert.Equal(t, tc.want, ev.Choice.Risk)
		})
	}
}

func TestParse_PermissionWithNoRiskTableDefaultsEverythingToSensitive(t *testing.T) {
	d := descriptorWithRisk(permissionMap(), nil)

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(`{"prompt_id":"p1","tool_name":"Bash"}`))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.RiskSensitive, ev.Choice.Risk,
		"a descriptor that declares no risk: table must never grant an implicit auto-approve")
}

func TestParse_QuestionChoiceCarriesNoRiskTier(t *testing.T) {
	d := descriptorWithRisk(permissionMap(), map[string][]string{"standard": {"AskUserQuestion"}})

	ev, err := inbound.Parse(d, spec.HookPermission, []byte(threeQuestionPayload))

	require.NoError(t, err)
	require.NotNil(t, ev.Choice)
	assert.Equal(t, models.ChoiceQuestion, ev.Choice.Kind)
	assert.Empty(t, ev.Choice.Risk,
		"a question prompt has no allow/deny to auto-resolve, so it must never carry a risk tier")
}

func descriptorWithRisk(fields map[string]string, risk map[string][]string) *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe", Events: map[string]spec.EventSpec{
		spec.HookPermission: {In: spec.HookPermission, Map: fields, Risk: risk},
	}}
	d.Runtime.Hooks.Format = "json"
	return d
}

func TestParse_AnAbsurdOptionListIsCapped(t *testing.T) {
	d := descriptor(map[string]map[string]string{spec.HookPermission: permissionMap()})
	options := make([]string, 0, 100)
	suggestions := make([]string, 0, 100)
	for i := range 100 {
		options = append(options, `{"label":"opt`+strconv.Itoa(i)+`"}`)
		suggestions = append(suggestions, `{"type":"sug`+strconv.Itoa(i)+`"}`)
	}

	question, err := inbound.Parse(d, spec.HookPermission, []byte(
		`{"tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"q",
		 "options":[`+strings.Join(options, ",")+`]}]}}`,
	))
	require.NoError(t, err)
	require.NotNil(t, question.Choice)
	require.Len(t, question.Choice.Questions, 1)
	assert.Len(t, question.Choice.Questions[0].Options, 32)

	permission, err := inbound.Parse(d, spec.HookPermission, []byte(
		`{"tool_name":"Bash","permission_suggestions":[`+strings.Join(suggestions, ",")+`]}`,
	))
	require.NoError(t, err)
	require.NotNil(t, permission.Choice)
	assert.Len(t, permission.Choice.Options, 34, "allow and deny, then the capped suggestions")
}
