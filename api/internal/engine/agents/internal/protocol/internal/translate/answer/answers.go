package answer

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	ErrNotAnswerable = errors.New("agents: provider declares no answer channel for this event")

	ErrUnsupportedDecision = errors.New("agents: provider cannot express this decision")

	ErrMalformedAnswer = errors.New("agents: rendered answer is not valid JSON")
)

const maxAnswerBytes = 256 << 10

func Capability(d *spec.Descriptor, canonical string) (models.AnswerCapability, bool) {
	if d == nil {
		return models.AnswerCapability{}, false
	}
	event, ok := d.AnswerFor(canonical)
	if !ok {
		return models.AnswerCapability{}, false
	}
	keys := make([]string, 0, len(event.Responses))
	for key, template := range event.Responses {
		if strings.TrimSpace(template) != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return models.AnswerCapability{}, false
	}
	sort.Strings(keys)
	return models.AnswerCapability{
		Wait: time.Duration(event.TimeoutSeconds) * time.Second,
		Keys: keys,
	}, true
}

func Render(
	d *spec.Descriptor,
	canonical string,
	raw []byte,
	decision models.AnswerDecision,
) ([]byte, error) {
	if d == nil {
		return nil, ErrNotAnswerable
	}
	event, ok := d.AnswerFor(canonical)
	if !ok {
		return nil, fmt.Errorf("%w: %q on %q", ErrNotAnswerable, canonical, d.ID)
	}
	template := event.Responses[decision.Key]
	if strings.TrimSpace(template) == "" {
		return nil, fmt.Errorf("%w: %q on %q", ErrUnsupportedDecision, decision.Key, d.ID)
	}

	out := replacer(d, event, canonical, raw, decision).Replace(template)
	if len(out) > maxAnswerBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds the %d-byte cap",
			ErrMalformedAnswer, len(out), maxAnswerBytes)
	}
	if !json.Valid([]byte(out)) {
		return nil, fmt.Errorf("%w on %q", ErrMalformedAnswer, d.ID)
	}
	return []byte(out), nil
}

func replacer(
	d *spec.Descriptor,
	event spec.AnswerEventSpec,
	canonical string,
	raw []byte,
	decision models.AnswerDecision,
) *strings.Replacer {
	return strings.NewReplacer(
		"{answers_json}", encode(decision.Answers, "{}"),
		"{reason_json}", encode(decision.Reason, `""`),
		"{content_json}", rawJSON(decision.Content, "{}"),
		"{tool_input_json}", toolInput(d, event, canonical, raw, decision.Answers),
	)
}

func toolInput(
	d *spec.Descriptor,
	event spec.AnswerEventSpec,
	canonical string,
	raw []byte,
	answers map[string]any,
) string {
	merged := declaredToolInput(d, canonical, raw)
	if event.AnswersInto != "" && len(answers) > 0 {
		merged[event.AnswersInto] = answers
	}
	return encode(merged, "{}")
}

func declaredToolInput(d *spec.Descriptor, canonical string, raw []byte) map[string]any {
	fields, declared := d.EventFields(canonical)
	if !declared || len(raw) == 0 {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{}
	}
	if sub := mapping.Object(decoded, fields["tool_input"]); sub != nil {
		return sub
	}
	return map[string]any{}
}

func encode(v any, fallback string) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(data)
}

func rawJSON(data []byte, fallback string) string {
	if len(data) == 0 || !json.Valid(data) {
		return fallback
	}
	return string(data)
}
