// Package answers renders a human's decision into the exact bytes a provider CLI
// expects to read back from its own hook.
//
// It is the mirror of package hooks. That one turns a provider payload into
// Crowbar's vocabulary; this one turns Crowbar's vocabulary into a provider's
// payload, from the same declarative descriptor and with the same rule — nothing
// here knows what any provider's JSON means. What it does know is that whatever
// it emits must be COMPLETE and VALID JSON, because a CLI reading half a document
// off a hook's stdout logs a validation failure and falls back to its own dialog.
package answers

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/payload"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	// ErrNotAnswerable reports a provider that declares no way to answer this
	// event's prompt. It is an ordinary outcome, not a failure: the caller falls
	// through and the CLI puts its own dialog up.
	ErrNotAnswerable = errors.New("agents: provider declares no answer channel for this event")
	// ErrUnsupportedDecision reports a decision key the provider has no template
	// for — a broader permission grant a CLI accepts through some channel this
	// descriptor does not declare, say. Refused rather than approximated.
	ErrUnsupportedDecision = errors.New("agents: provider cannot express this decision")
	// ErrMalformedAnswer reports a rendered answer that is not valid JSON. It can
	// only mean a mis-authored template, and emitting it would be worse than
	// emitting nothing: the CLI would log a parse failure and the human's decision
	// would be silently lost.
	ErrMalformedAnswer = errors.New("agents: rendered answer is not valid JSON")
)

// maxAnswerBytes bounds one rendered answer. The relay prints this on a hook's
// stdout, which a CLI reads into memory before parsing; an unbounded echo of a
// tool input would be a way to make a provider read whatever a payload contained.
const maxAnswerBytes = 256 << 10

// Capability reports what a provider can be told about one canonical event's
// prompt. ok=false means it can be told nothing, which is the answer for every
// provider that declares no `answer:` block at all.
func Capability(d *spec.Descriptor, canonical string) (models.AnswerCapability, bool) {
	if d == nil {
		return models.AnswerCapability{}, false
	}
	event, ok := d.Answer.Event(canonical)
	if !ok {
		return models.AnswerCapability{}, false
	}
	keys := make([]string, 0, len(event.Responses))
	for key, template := range event.Responses {
		// An empty template is a key that was declared and then left blank. It
		// renders nothing, so it is not a decision this provider can express, and
		// counting it would let the caller hold a hook open for an answer it could
		// never print.
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

// Render produces the bytes the relay must print on the hook's stdout.
//
// raw is the ORIGINAL hook payload the prompt arrived on, because some answers
// are not a verdict but an edit of what the provider sent: claude answers
// AskUserQuestion by being handed its own tool input back with the picks added
// (measured against 2.1.234 on 2026-08-18). Reading it here rather than storing
// it on the prompt record is what keeps the record unchanged by this whole
// channel.
func Render(
	d *spec.Descriptor,
	canonical string,
	raw []byte,
	decision models.AnswerDecision,
) ([]byte, error) {
	if d == nil {
		return nil, ErrNotAnswerable
	}
	event, ok := d.Answer.Event(canonical)
	if !ok {
		return nil, fmt.Errorf("%w: %q on %q", ErrNotAnswerable, canonical, d.ID)
	}
	template := event.Responses[decision.Key]
	if strings.TrimSpace(template) == "" {
		return nil, fmt.Errorf("%w: %q on %q", ErrUnsupportedDecision, decision.Key, d.ID)
	}

	// ONE pass, deliberately. A replacement is data — an echoed tool input is
	// whatever the provider sent — so a second pass could expand a placeholder that
	// arrived inside a payload, and with map iteration order deciding which. A
	// single left-to-right scan never re-reads what it has written.
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

// replacer builds every placeholder's expansion. Each one is a complete JSON
// VALUE, so a template that was valid JSON with the placeholders in place is
// still valid JSON once they are replaced — which is what lets a mis-authored
// template be caught by json.Valid rather than by a CLI at 3am.
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

// toolInput echoes the gated tool's own input back with the human's picks merged
// in under the descriptor's declared key.
//
// The tool input's PATH is read from the same hooks mapping that observed the
// prompt, so this package learns nothing about a provider's payload shape that
// the read side did not already declare. A payload that cannot be decoded, or a
// descriptor that mapped no tool input, yields an object holding only the answers
// — never a fragment, and never a silent drop of the decision.
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

// declaredToolInput reads the tool-input subtree the READ side already declared,
// or an empty object when there is none to read.
func declaredToolInput(d *spec.Descriptor, canonical string, raw []byte) map[string]any {
	fields, declared := d.Hooks.Event(canonical)
	if !declared || len(raw) == 0 {
		return map[string]any{}
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return map[string]any{}
	}
	if sub := payload.Object(decoded, fields["tool_input"]); sub != nil {
		return sub
	}
	return map[string]any{}
}

// encode marshals a value, falling back to a literal when it cannot be encoded.
// The fallback is a JSON value of the right shape rather than an error, because
// every caller here is building one fragment of a document that must stay
// parseable.
func encode(v any, fallback string) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(data)
}

// rawJSON passes through bytes that are already JSON, and substitutes the
// fallback for anything else. A caller's free-form content is never re-encoded:
// re-encoding an elicitation's filled-in form would change the very document the
// MCP server asked for.
func rawJSON(data []byte, fallback string) string {
	if len(data) == 0 || !json.Valid(data) {
		return fallback
	}
	return string(data)
}
