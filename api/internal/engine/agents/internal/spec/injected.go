package spec

// Canonical injected-prompt kinds — Crowbar's own words for a prompt a provider's
// HARNESS wrote and fed to its own model through the user-prompt channel.
//
// The set is deliberately tiny and grows only when a payload specific enough to
// justify a name has been captured from a real CLI. A declaration that can only
// say "this did not come from the human" names no kind and is recorded that way,
// which is the honest answer: the ledger needs to know whose words these are, and
// it does not need to know what the harness was talking about.
const (
	// InjectedPromptTaskNotification is claude's background-subagent completion
	// report — the `<task-notification>` document it hands its own model when a
	// task launched with the Agent tool finishes.
	InjectedPromptTaskNotification = "task_notification"
)

// InjectedPromptKinds is the closed set a descriptor may name. Empty is always
// allowed and means "injected, unidentified".
var InjectedPromptKinds = map[string]struct{}{
	InjectedPromptTaskNotification: {},
}

// InjectedPromptSpec is one prompt a provider's own harness submits through the
// user-prompt channel, as if a human had typed it.
//
// It exists because that channel carries two completely different things and the
// payload does not say which. Crowbar's ledger recorded every one of them as the
// user's, so a chat's record contained sentences its user never wrote — and
// get_chat_log then served those sentences to OTHER agents as things the human
// had said.
//
// THIS IS A TEXT MATCH, AND A TEXT MATCH IS WEAKER THAN A DISCRIMINATOR. It reads
// the prompt's own body and decides from a declared string, which is guessing
// from content — the thing a structural field exists to make unnecessary. It is
// here because there is no structural field to read.
//
// Measured against claude 2.1.234 on 2026-08-18 by capturing raw hook stdin from
// live interactive PTY sessions through a throwaway --settings file. Its
// documented UserPromptSubmit schema declares
//
//	source: ("user"|"sdk"|"system"|"loop_wakeup"|"schedule_wakeup"|"poll_event").optional()
//
// and `source` was ABSENT on all three paths that reach Crowbar: Crowbar's own
// delivery (the prompt as a positional arg after `--`), a human typing into
// claude's composer and pressing Enter, and the harness's `<task-notification>`
// subagent-completion report. All three carried an IDENTICAL key set, with no key
// in either direction to tell them apart:
//
//	session_id, transcript_path, cwd, prompt_id, permission_mode,
//	hook_event_name, prompt
//
// So a rule of "record only when source == user" would drop every real message
// the user sent, and its inverse would attribute every real message to the
// harness. There is nothing else in the payload to ask.
//
// THE MOMENT A PROVIDER EMITS A REAL DISCRIMINATOR, THIS SHOULD SWITCH TO IT and
// these declarations should go. A field the provider populates is authoritative
// about its own payload in a way that reading the body can never be, and a
// version of claude that starts sending `source` makes this whole block dead
// weight that can still get a human's message wrong.
//
// Until then the declarations live HERE rather than in Go, for the same reason
// the terminal-prompt needles do (see TerminalPromptSpec): they are provider
// vocabulary, and provider vocabulary never belongs in code in this codebase. A
// CLI release that renames its notification document is then a descriptor edit,
// on disk, with no daemon build.
//
// A provider that declares NOTHING is unaffected in every observable way: no
// prompt of its ever matches, and every user_prompt it fires is recorded as the
// user's exactly as it was before this file existed. Absent means the user, and
// that default is what protects both the human typing into the PTY and Crowbar's
// own delivery path.
type InjectedPromptSpec struct {
	// Kind names the injection when this needle identifies one specifically. It
	// must be empty or a member of InjectedPromptKinds; a descriptor naming a kind
	// the daemon does not know is rejected rather than silently downgraded,
	// because a typo would otherwise read as a working declaration forever.
	Kind string `yaml:"kind"`

	// Needle is the literal text the harness's document OPENS with, compared
	// against the raw prompt byte for byte after leading whitespace is trimmed
	// (see the promptorigin package).
	//
	// Anchored at the start, and NOT reduced the way a terminal needle is: this is
	// raw hook text, not a screen a TUI has boxed and wrapped, so there is no
	// padding to see through — and the characters a screen reduction throws away
	// are the angle brackets that carry every bit of meaning here.
	Needle string `yaml:"needle"`
}
