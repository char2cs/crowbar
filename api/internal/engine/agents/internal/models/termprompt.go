package models

// TerminalPrompt is a provider CLI caught blocking on a modal Crowbar cannot
// answer: the workspace-trust dialog and its relatives, which travel through no
// hook and therefore leave a chat pane showing nothing at all.
//
// It is a POSITIVE identification of a blocked CLI, never a guess about an idle
// one. The engine only ever produces it from a string a descriptor declared.
type TerminalPrompt struct {
	// Kind is Crowbar's own name for a prompt it recognises specifically, or ""
	// when all that is known is that the CLI is blocked on something. A client
	// renders the empty case generically — "waiting for input in the terminal" —
	// because naming a prompt we have not identified is worse than not naming it.
	Kind string

	// Needle is the declared string that matched. It is evidence, not display
	// text: it is a provider's vocabulary, so it belongs in a log or a test
	// failure and never in the UI.
	Needle string
}
