package domain

// AgentTerminalWait is the answer to "is this chat's CLI blocked on something
// only the terminal can clear?".
//
// It exists because that state is otherwise INVISIBLE. A provider CLI can stop on
// a modal that travels through no hook — the workspace-trust dialog is the
// standing example, and first-run setup, login and migration prompts are the same
// shape — and while it does, the chat has no open turn, no pending prompt and no
// error to show. The pane renders nothing, indistinguishable from a healthy idle
// chat, and the user reasonably concludes Crowbar is broken.
//
// It is DERIVED, never stored: it is recomputed from a live runner, the chat's own
// busy state, its outstanding prompts, and the daemon's screen model. Nothing
// persists it, and a daemon restart simply computes it again.
//
// The zero value is the ordinary answer — "no, nothing is blocked" — which is what
// every chat of a provider declaring no needles reports forever.
type AgentTerminalWait struct {
	// Waiting is the whole verdict. False means either that nothing matched or
	// that one of the gates said this question does not apply to this chat right
	// now; the two are the same answer to a client and are deliberately not
	// distinguished.
	Waiting bool

	// Kind is Crowbar's own name for the prompt when it recognises one
	// specifically — domain-neutral, never a provider's word. Empty means only
	// that something is up, and a client says exactly that rather than guessing.
	Kind string
}

// AgentTerminalWaitKind values. The set is small on purpose: a kind is added when
// a needle specific enough to justify naming has been captured from a real CLI,
// and never before.
const (
	// AgentTerminalWaitTrust is the first-run "do you trust this folder?" dialog.
	AgentTerminalWaitTrust = "workspace_trust"
)
