package models

import "time"

// AnswerCapability is what a provider can be told about one canonical event's
// prompt, and how long it will wait to be told it.
//
// A caller uses it for exactly one decision: whether to HOLD the relay open on
// this hook. Absent capability means the hook returns immediately and the
// provider falls through to its own dialog, which is what happened before an
// answer channel existed.
type AnswerCapability struct {
	// Wait is how long the daemon may hold the hook. It is the descriptor's
	// declared budget, and it is deliberately not the provider's own hook timeout:
	// the budget has to expire FIRST so the relay exits cleanly under Crowbar's
	// control rather than being killed mid-write.
	Wait time.Duration
	// Keys are the decision keys this event accepts, sorted. A key outside them
	// cannot be rendered and must be refused rather than approximated.
	Keys []string
}

// Accepts reports whether a decision key can be rendered for this event.
func (c AnswerCapability) Accepts(key string) bool {
	for _, k := range c.Keys {
		if k == key {
			return true
		}
	}
	return false
}

// AnswerDecision is what a human decided, in Crowbar's own vocabulary.
//
// It carries no provider shape at all: which JSON a CLI wants to read is the
// descriptor's business, and this type is what the descriptor's template is
// rendered against.
type AnswerDecision struct {
	// Key selects the response template — the chosen option's Kind for a prompt
	// that offers options, and the option id itself for one that offers none.
	Key string
	// Answers maps the question's text to what was picked: a single label, or the
	// list of labels when the prompt allowed more than one.
	Answers map[string]any
	// Reason is the human's own words, carried where the provider accepts them
	// (claude's deny `message`). Empty is ordinary.
	Reason string
	// Content is a free-form answer document for a prompt whose answer is a FORM
	// rather than a pick — an MCP elicitation's filled-in schema. Raw JSON,
	// uninterpreted.
	Content []byte
}
