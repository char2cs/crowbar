package answers_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/answers"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// answering is a descriptor shaped exactly like claude's: the permission event
// maps a tool_input path on the read side, and the answer block renders three
// decisions back out through it.
//
// The response templates are the shapes MEASURED against claude 2.1.234 on
// 2026-08-18. The hookSpecificOutput wrapper is not decoration — a bare
// {"decision":{...}} was measured failing the CLI's own output validator with
// `Hook JSON output validation failed — (root): Invalid input`, after which the
// TUI dialog was drawn and the hook's decision was discarded.
func answering() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "vendor",
		Hooks: spec.HookSpec{
			Format: "json",
			Events: map[string]map[string]string{
				"permission": {"tool_name": "tool_name", "tool_input": "tool_input"},
			},
		},
		Answer: spec.AnswerSpec{
			"permission": {
				TimeoutSeconds: 270,
				AnswersInto:    "answers",
				Responses: map[string]string{
					"allow": `{"hookSpecificOutput":{"hookEventName":"PermissionRequest",` +
						`"decision":{"behavior":"allow"}}}`,
					"deny": `{"hookSpecificOutput":{"hookEventName":"PermissionRequest",` +
						`"decision":{"behavior":"deny","message":{reason_json}}}}`,
					"answer": `{"hookSpecificOutput":{"hookEventName":"PermissionRequest",` +
						`"decision":{"behavior":"allow","updatedInput":{tool_input_json}}}}`,
				},
			},
			"elicitation": {
				TimeoutSeconds: 270,
				Responses: map[string]string{
					"accept": `{"hookSpecificOutput":{"hookEventName":"Elicitation",` +
						`"action":"accept","content":{content_json}}}`,
				},
			},
		},
	}
}

// askUserQuestionPayload is the PermissionRequest captured live from claude
// 2.1.234 on 2026-08-18 for an AskUserQuestion call.
func askUserQuestionPayload() []byte {
	return []byte(`{
	  "session_id":"s1","prompt_id":"2819fe04","hook_event_name":"PermissionRequest",
	  "tool_name":"AskUserQuestion",
	  "tool_input":{"questions":[{"question":"Which option do you prefer?","header":"Choice",
	    "options":[{"label":"Option A"},{"label":"Option B"}],"multiSelect":false}]}
	}`)
}

func TestCapability_AbsentBlockIsNotAnswerable(t *testing.T) {
	// A descriptor that declares nothing is the whole degradation story: no relay is
	// ever held, and every hook behaves as it did before an answer channel existed.
	_, ok := answers.Capability(&spec.Descriptor{ID: "silent"}, "permission")
	assert.False(t, ok)

	_, ok = answers.Capability(nil, "permission")
	assert.False(t, ok, "a nil descriptor must answer 'not answerable', never panic")
}

func TestCapability_UndeclaredEventIsNotAnswerable(t *testing.T) {
	_, ok := answers.Capability(answering(), "tool_pre")
	assert.False(t, ok)
}

func TestCapability_ReportsSortedKeysAndTheDeclaredBudget(t *testing.T) {
	capability, ok := answers.Capability(answering(), "permission")
	require.True(t, ok)
	assert.Equal(t, []string{"allow", "answer", "deny"}, capability.Keys)
	assert.Equal(t, 270*time.Second, capability.Wait)
	assert.True(t, capability.Accepts("deny"))
	assert.False(t, capability.Accepts("suggestion"),
		"a decision with no template must not be reported as expressible")
}

// A key declared and left blank renders nothing, so counting it would hold a
// relay open for an answer that could never be printed.
func TestCapability_BlankTemplatesAreNotKeys(t *testing.T) {
	d := answering()
	d.Answer["permission"] = spec.AnswerEventSpec{
		TimeoutSeconds: 5,
		Responses:      map[string]string{"allow": "   ", "deny": ""},
	}
	_, ok := answers.Capability(d, "permission")
	assert.False(t, ok, "a block whose every template is blank declares nothing")
}

func TestCapability_EmptyResponseMapIsNotAnswerable(t *testing.T) {
	d := answering()
	d.Answer["permission"] = spec.AnswerEventSpec{TimeoutSeconds: 5}
	_, ok := answers.Capability(d, "permission")
	assert.False(t, ok)
}

func TestRender_AllowIsTheProvidersOwnWrappedShape(t *testing.T) {
	out, err := answers.Render(answering(), "permission", nil,
		models.AnswerDecision{Key: "allow"})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"hookSpecificOutput":{"hookEventName":"PermissionRequest",`+
			`"decision":{"behavior":"allow"}}}`,
		string(out))
}

func TestRender_DenyCarriesTheHumansWordsAsAJSONString(t *testing.T) {
	out, err := answers.Render(answering(), "permission", nil, models.AnswerDecision{
		Key: "deny", Reason: `no "rm -rf" here` + "\n",
	})
	require.NoError(t, err)
	// The reason is a QUOTED value, not a spliced fragment: a decision carrying a
	// quote or a newline must not be able to break the document it travels in.
	var decoded struct {
		HookSpecificOutput struct {
			Decision struct {
				Message string `json:"message"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal(out, &decoded))
	assert.Equal(t, `no "rm -rf" here`+"\n", decoded.HookSpecificOutput.Decision.Message)
}

// The measured way to answer AskUserQuestion: hand the tool its own input back
// with an `answers` object keyed by the question TEXT and valued with the chosen
// option's LABEL.
func TestRender_AnswerEchoesTheToolInputWithThePicksMergedIn(t *testing.T) {
	out, err := answers.Render(answering(), "permission", askUserQuestionPayload(),
		models.AnswerDecision{
			Key:     "answer",
			Answers: map[string]any{"Which option do you prefer?": "Option A"},
		})
	require.NoError(t, err)

	var decoded struct {
		HookSpecificOutput struct {
			Decision struct {
				Behavior     string `json:"behavior"`
				UpdatedInput struct {
					Questions []map[string]any  `json:"questions"`
					Answers   map[string]string `json:"answers"`
				} `json:"updatedInput"`
			} `json:"decision"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal(out, &decoded))
	updated := decoded.HookSpecificOutput.Decision.UpdatedInput
	assert.Equal(t, "allow", decoded.HookSpecificOutput.Decision.Behavior)
	require.Len(t, updated.Questions, 1, "the provider's own question must survive the echo")
	assert.Equal(t, "Option A", updated.Answers["Which option do you prefer?"])
}

func TestRender_AnswerWithNoReadablePayloadStillCarriesThePicks(t *testing.T) {
	// A payload that cannot be decoded loses the echo, not the decision. Emitting a
	// fragment, or dropping the answer silently, would both be worse.
	out, err := answers.Render(answering(), "permission", []byte("not json"),
		models.AnswerDecision{Key: "answer", Answers: map[string]any{"q": "a"}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"answers":{"q":"a"}`)
}

func TestRender_ElicitationContentIsPassedThroughUninterpreted(t *testing.T) {
	out, err := answers.Render(answering(), "elicitation", nil, models.AnswerDecision{
		Key: "accept", Content: []byte(`{"choice":"B","note":"kept verbatim"}`),
	})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"content":{"choice":"B","note":"kept verbatim"}`)
}

func TestRender_ElicitationWithUnusableContentFallsBackToAnEmptyObject(t *testing.T) {
	out, err := answers.Render(answering(), "elicitation", nil, models.AnswerDecision{
		Key: "accept", Content: []byte("{oops"),
	})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"content":{}`)
}

func TestRender_RefusesADecisionTheProviderCannotExpress(t *testing.T) {
	_, err := answers.Render(answering(), "permission", nil,
		models.AnswerDecision{Key: "suggestion"})
	require.ErrorIs(t, err, answers.ErrUnsupportedDecision)
}

func TestRender_RefusesAnEventWithNoAnswerChannel(t *testing.T) {
	_, err := answers.Render(answering(), "tool_pre", nil, models.AnswerDecision{Key: "allow"})
	require.ErrorIs(t, err, answers.ErrNotAnswerable)

	_, err = answers.Render(nil, "permission", nil, models.AnswerDecision{Key: "allow"})
	require.ErrorIs(t, err, answers.ErrNotAnswerable)
}

// A mis-authored template must fail HERE. Printing invalid JSON on a hook makes
// the CLI log a parse failure and fall back to its dialog, which loses the
// human's decision with no trace of why.
func TestRender_RefusesToEmitInvalidJSON(t *testing.T) {
	d := answering()
	d.Answer["permission"] = spec.AnswerEventSpec{
		TimeoutSeconds: 5,
		Responses:      map[string]string{"allow": `{"decision":`},
	}
	_, err := answers.Render(d, "permission", nil, models.AnswerDecision{Key: "allow"})
	require.ErrorIs(t, err, answers.ErrMalformedAnswer)
}

func TestRender_RefusesAnOversizedAnswer(t *testing.T) {
	huge := map[string]any{"questions": strings.Repeat("x", 300<<10)}
	payload, err := json.Marshal(map[string]any{"tool_input": huge})
	require.NoError(t, err)

	_, err = answers.Render(answering(), "permission", payload,
		models.AnswerDecision{Key: "answer", Answers: map[string]any{"q": "a"}})
	require.ErrorIs(t, err, answers.ErrMalformedAnswer)
}

// Placeholders are expanded in ONE left-to-right pass. A payload that happens to
// contain a placeholder's own text is data, and a second pass would expand it —
// with map iteration order deciding whether it did.
func TestRegression_APayloadCannotSmuggleItsOwnPlaceholder(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "echo {reason_json}"},
	})
	require.NoError(t, err)

	d := answering()
	d.Answer["permission"] = spec.AnswerEventSpec{
		TimeoutSeconds: 5,
		AnswersInto:    "answers",
		Responses: map[string]string{
			"answer": `{"input":{tool_input_json},"reason":{reason_json}}`,
		},
	}
	out, err := answers.Render(d, "permission", payload, models.AnswerDecision{
		Key: "answer", Reason: "SMUGGLED", Answers: map[string]any{"q": "a"},
	})
	require.NoError(t, err)

	var decoded struct {
		Input struct {
			Command string `json:"command"`
		} `json:"input"`
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(out, &decoded))
	assert.Equal(t, "echo {reason_json}", decoded.Input.Command,
		"the payload's text must reach the provider unchanged")
	assert.Equal(t, "SMUGGLED", decoded.Reason)
}

// A descriptor that maps no tool_input path still answers — with the picks alone.
func TestRender_AnswerWithNoDeclaredToolInputPathCarriesOnlyThePicks(t *testing.T) {
	d := answering()
	d.Hooks.Events["permission"] = map[string]string{"tool_name": "tool_name"}
	out, err := answers.Render(d, "permission", askUserQuestionPayload(),
		models.AnswerDecision{Key: "answer", Answers: map[string]any{"q": "a"}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"updatedInput":{"answers":{"q":"a"}}`)
}

// A decision carrying something that cannot be encoded still yields a PARSEABLE
// document. Every expansion here is one fragment of a JSON document that a
// provider is about to read, so the fallback has to be a value of the right
// shape — an error at this point would lose the decision with nothing to show
// for it, and a bare fragment would corrupt the whole answer.
func TestRender_AnUnencodableAnswerFallsBackToAnEmptyObject(t *testing.T) {
	d := answering()
	d.Answer["permission"] = spec.AnswerEventSpec{
		TimeoutSeconds: 5,
		AnswersInto:    "answers",
		Responses: map[string]string{
			"answer": `{"picks":{answers_json},"input":{tool_input_json}}`,
		},
	}
	out, err := answers.Render(d, "permission", nil, models.AnswerDecision{
		Key: "answer", Answers: map[string]any{"q": make(chan int)},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"picks":{},"input":{}}`, string(out))
}
