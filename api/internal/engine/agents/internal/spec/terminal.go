package spec

// Canonical terminal-prompt kinds — Crowbar's own words for a blocking modal it
// RECOGNISES, never a provider's.
//
// The set is deliberately tiny, and it grows only when a needle specific enough
// to justify a name has been captured from a real CLI. A prompt Crowbar can only
// detect, not identify, declares no kind at all and is reported generically:
// saying "waiting for input in the terminal" is honest, and saying "waiting for
// workspace trust" over a login prompt is not.
const (
	// TerminalPromptTrust is the first-run "do you trust this folder?" dialog.
	TerminalPromptTrust = "workspace_trust"
)

// TerminalPromptKinds is the closed set a descriptor may name. Empty is always
// allowed and means "detected, unidentified".
var TerminalPromptKinds = map[string]struct{}{
	TerminalPromptTrust: {},
}

// TerminalPromptSpec is one string a provider CLI paints while it is BLOCKING on
// a modal that reaches Crowbar through NO hook — the workspace-trust dialog is
// the standing example, and first-run setup, login and migration prompts are the
// same shape.
//
// It exists because that state is otherwise indistinguishable from a healthy idle
// chat: no spinner, no message, no error, a pane that looks dead. Crowbar cannot
// answer these prompts, so the only correct behaviour is to say so and get the
// user to the terminal — which it can only do if it knows.
//
// The strings live HERE rather than in Go because they are provider vocabulary,
// and provider vocabulary never belongs in code in this codebase. A CLI release
// that repaints its dialog is then a descriptor edit, on disk, with no daemon
// build.
type TerminalPromptSpec struct {
	// Kind names the prompt when this needle identifies one specifically. It must
	// be empty or a member of TerminalPromptKinds; a descriptor naming a kind the
	// daemon does not know is rejected rather than silently downgraded, because a
	// typo would otherwise read as "we only know that something is up" forever.
	Kind string `yaml:"kind"`

	// Needle is the literal screen text. Matching is whitespace- and
	// punctuation-insensitive (see the termprompt package), so a needle survives
	// the box drawing, padding and line wrapping a TUI puts around it.
	Needle string `yaml:"needle"`
}
