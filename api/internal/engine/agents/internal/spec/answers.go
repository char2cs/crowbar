package spec

// AnswerSpec is the WRITE side of a prompt: how a decision taken in Crowbar
// reaches the CLI that is blocked waiting for it.
//
// It is keyed by CANONICAL HOOK KIND because the answer travels back out through
// the very hook that asked. A provider CLI blocks its own gate while the hook
// process runs and reads that process's stdout as the decision, so the only
// channel a prompt can be answered on is the one it arrived on. Measured against
// claude 2.1.234 on 2026-08-18: a PermissionRequest hook held the dialog and then
// answered it, and the AskUserQuestion the same hook was gating came back
// answered in the PostToolUse payload — no dialog, no keystroke.
//
// A provider that declares NOTHING here is unaffected in every observable way:
// no prompt it opens is answerable, the relay never blocks, and every hook it
// fires behaves byte-for-byte as it did before this file existed. That is the
// degradation story, and it is why answerability is declared rather than
// inferred.
type AnswerSpec map[string]AnswerEventSpec

// AnswerEventSpec declares how one canonical event's prompt is answered.
type AnswerEventSpec struct {
	// TimeoutSeconds is how long the DAEMON will hold the hook waiting for a
	// human. It must be comfortably SHORTER than the timeout injected on the
	// provider's own hook registration, so the relay exits under Crowbar's control
	// instead of being killed mid-write: measured against claude 2.1.234, a hook
	// that overruns its declared `timeout` is KILLED, not abandoned.
	TimeoutSeconds int `yaml:"timeout_seconds"`

	// AnswersInto is the key an answered QUESTION's picks are merged into, inside
	// the echoed tool input. Claude's AskUserQuestion is answered by handing the
	// tool its own input back with an `answers` object added — a map from the
	// question's text to the chosen option's label (measured 2026-08-18; the CLI
	// logs `Hook satisfied user interaction for AskUserQuestion via updatedInput`).
	// A provider that answers questions some other way names something else, or
	// nothing.
	AnswersInto string `yaml:"answers_into"`

	// Responses maps a DECISION KEY to the exact JSON the relay must print.
	//
	// The key is Crowbar's own vocabulary — the chosen option's Kind for a prompt
	// that offers options (allow / deny / answer / suggestion), and the option id
	// itself for a prompt that offers none, which is how an elicitation's
	// accept/decline/cancel is named. A key with no template here is not
	// answerable, and the answer is refused rather than approximated: printing
	// "allow" for a broader grant the user actually asked for would grant less than
	// they chose while telling them it worked.
	//
	// Placeholders, all of which expand to a complete JSON VALUE (never a bare
	// fragment), so a template is valid JSON before and after expansion:
	//
	//	{tool_input_json}  the gated tool's own input, with AnswersInto added
	//	{answers_json}     just that answers object
	//	{reason_json}      the human's reason, as a JSON string ("" when none)
	//	{content_json}     a free-form answer document, as JSON ({} when none)
	Responses map[string]string `yaml:"responses"`
}

// Event returns the answer spec for a canonical kind, and whether the provider
// declares one at all.
func (a AnswerSpec) Event(canonical string) (AnswerEventSpec, bool) {
	s, ok := a[canonical]
	if !ok || len(s.Responses) == 0 {
		// A block with no response templates declares nothing answerable. Treating it
		// as absent rather than as present-but-empty is what keeps "the relay blocks"
		// and "the relay can print something" the same condition — a hook held open
		// for a decision that could never be rendered is strictly worse than one that
		// falls straight through to the provider's own dialog.
		return AnswerEventSpec{}, false
	}
	return s, true
}
