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

// Canonical terminal-notice kinds — Crowbar's own words for a message a CLI
// paints INSTEAD of finishing what it was asked to do.
//
// A notice is not a prompt. Nobody is being asked anything and there is nothing
// to answer: the CLI has already given up, said why on its own screen, and gone
// quiet. The set is as small as the prompt set and grows on the same terms —
// only from a string captured off a real CLI in the state it describes.
const (
	// TerminalNoticeUsageLimit is a provider refusing to run because the
	// account is out of quota. Measured on codex-cli 0.146.0, which paints its
	// banner and then simply sits there: no Stop hook, no exit, no error —
	// which is precisely why this vocabulary had to exist.
	TerminalNoticeUsageLimit = "usage_limit"
)

// TerminalNoticeKinds is the closed set a descriptor may name. Unlike
// TerminalPromptKinds, empty is NOT allowed — see TerminalNoticeSpec.Kind.
var TerminalNoticeKinds = map[string]struct{}{
	TerminalNoticeUsageLimit: {},
}

// TerminalNoticeSpec is one string a provider CLI paints when an attempt ENDED
// for a reason it reports on its screen and nowhere else.
//
// It is the third member of a family. `answer:` covers prompts that arrive over
// a hook and can be decided from the chat. TerminalPromptSpec covers prompts that
// arrive nowhere and can only be cleared by a human at the terminal. This covers
// the case where nothing is being asked at all: the CLI took the turn, could not
// run it, wrote a sentence explaining why, and then neither finished the turn nor
// died — so no hook fires, the process stays alive, and the runner-exit reconcile
// that closes an abandoned turn is never reached either.
//
// Measured against codex-cli 0.146.0. A prompt was accepted, `user_prompt` fired,
// the turn opened, and the CLI then painted its usage-limit banner and stopped.
// The turn stayed open for 44 minutes — until the user manually switched provider
// and the displace closed it. Nothing else in the daemon could have.
//
// The strings live HERE, in the descriptor, for the same reason the prompt
// needles do: they are provider vocabulary, and a CLI release that repaints its
// banner must be a YAML edit rather than a daemon build.
type TerminalNoticeSpec struct {
	// Kind names the notice. It is REQUIRED, and must be a member of
	// TerminalNoticeKinds — which is the one place this type is stricter than
	// TerminalPromptSpec, deliberately.
	//
	// A prompt needle only ever produces a banner, so an unidentified one can
	// honestly report "something is up". A notice with EndsTurn CLOSES A TURN,
	// and closing a turn is an assertion that a live process has stopped
	// working. That assertion may not be reachable by writing a new string into
	// a YAML file: adding one obliges somebody to add a kind here too, in Go,
	// where the reviewer has to look at what is being claimed.
	Kind string `yaml:"kind"`

	// Needle is the literal screen text. Matching is whitespace- and
	// punctuation-insensitive and spans screen rows (see the termprompt
	// package), so a needle survives the wrapping a TUI applies at whatever
	// width the pane happens to be.
	Needle string `yaml:"needle"`

	// EndsTurn declares that this notice is painted BECAUSE the attempt ended.
	//
	// That is the whole reason a notice can be corroborating evidence where an
	// "idle composer" needle cannot. Neither shipped CLI has a usable idle
	// needle: codex's composer placeholder ROTATES between sessions
	// (`Implement {feature}`, `Use /skills to list available skills`,
	// `Explain this codebase` — three consecutive fresh sessions, same
	// directory), and claude's footer hint is painted WHILE IT IS GENERATING —
	// `⏵⏵ auto mode on (shift+tab to cycle)` was on screen with assistant prose
	// visibly streaming, and it is mode- and model-dependent besides. Declaring
	// either as "this CLI is idle" would darken a working agent's spinner,
	// which is worse than the wedge this whole mechanism exists to end.
	//
	// A notice WITHOUT it is a message that means nothing about liveness. None
	// is declared today; the field is a flag rather than an implication so a
	// future informational needle cannot silently acquire the power to close a
	// turn by being added to this list.
	EndsTurn bool `yaml:"ends_turn"`
}
