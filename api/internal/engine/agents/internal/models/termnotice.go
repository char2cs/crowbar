package models

// TerminalNotice is a provider CLI caught explaining, on its own screen and
// nowhere else, that it is not going to do what it was just asked to do.
//
// It is the counterpart of TerminalPrompt, and the difference between them is who
// is waiting for whom. A prompt is the CLI waiting on a human. A notice is the
// CLI having already finished — badly — and said so in a sentence it painted
// instead of firing the hook that would have closed the turn.
//
// The measured instance is codex-cli 0.146.0 out of quota: it accepts the prompt,
// fires user_prompt, paints its usage-limit banner, and then neither completes the
// turn nor exits. No Stop hook, no SessionEnd, a live process — so every mechanism
// Crowbar has for closing a turn is out of reach at once, and the chat spins until
// a human intervenes.
type TerminalNotice struct {
	// Kind is Crowbar's own name for the notice. Never empty: a descriptor
	// cannot declare a notice without one (see spec.TerminalNoticeSpec.Kind),
	// because acting on an unidentified notice is the failure this whole
	// mechanism is built to avoid.
	Kind string

	// Needle is the declared string that matched. Evidence, not display text —
	// it belongs in a log or a test failure, never in the UI.
	Needle string

	// Text is what the CLI ACTUALLY WROTE: the matched screen row plus the rows
	// its sentence wrapped onto, right-trimmed and rejoined.
	//
	// This is the only field that reaches a user, and it is verbatim on purpose.
	// The provider's own sentence carries the part Crowbar could never generate —
	// codex's banner names the plan to upgrade to, the URL to buy credits at, and
	// the exact time the limit resets. A Crowbar paraphrase would drop all three
	// and would be a second, worse account of something the CLI already said
	// perfectly well.
	Text string

	// EndsTurn is the declaration that this notice is painted BECAUSE the attempt
	// ended — so its presence on a quiescent screen is positive evidence that the
	// CLI is not working, rather than the absence of evidence that it is.
	EndsTurn bool
}
