// Package promptsigil keeps Crowbar's own attachment encoding from reading as
// a CLI's control gesture, and undoes that again on the way back.
//
// A vendor CLI decides what a message MEANS from its first character: both
// shipped ones open a shell mode on `!`. Crowbar writes an image attachment as
// `![alt](chats/<chatID>/attachments/<file>)`, so something attached before
// anything was typed puts that character at the front of the message with
// nobody having typed it. Measured on codex-cli 0.149.1: such a message was
// RUN as `[screenshot.png](/…/shot.png)` and never reached the model — and,
// because no prompt event ever fired, never became a ledger turn either. It
// simply vanished.
//
// WHICH characters those are, and what hides one, is the descriptor's to say
// (presentation.prompt_submit.leading_sigils) — nothing here knows `!`.
package promptsigil

import (
	"regexp"
	"slices"
	"strings"
)

// leadingAttachmentPattern matches a message OPENING with a durable attachment
// reference, in either the image form `![alt](…)` — whose own first character
// is the sigil this package exists for — or the plain link form `[name](…)`.
// Same segment rules as the dispatch rewrite's own pattern: a chatID and a
// filename with no "/" in either, so neither can walk out of the store.
var leadingAttachmentPattern = regexp.MustCompile(
	`^!?\[[^\]\n]*\]\(chats/([^/\s)]+)/attachments/[^/)\s]+\)`,
)

// leadsWithAttachmentRef reports whether text opens with chatID's OWN durable
// attachment reference. A reference naming a different chat is not ours to
// reason about, and is left alone.
func leadsWithAttachmentRef(chatID, text string) bool {
	match := leadingAttachmentPattern.FindStringSubmatch(text)
	return match != nil && match[1] == chatID
}

func opensWithASigil(chars []string, text string) bool {
	return slices.ContainsFunc(chars, func(char string) bool { return strings.HasPrefix(text, char) })
}

// Guard returns a copy of text whose first character is no longer one of this
// provider's sigils WHEN that character is one Crowbar itself wrote — the `!`
// of its own leading image attachment. Everything else passes through
// byte-identical, which is the whole point: a `!` a PERSON typed still opens
// shell mode, and the `/compact` Crowbar sends down this same path as prompt
// text is still a slash command.
//
// Idempotent. Applied twice, the second call sees escape's own first character
// (not a sigil) and does nothing.
func Guard(chars []string, escape, chatID, text string) string {
	if len(chars) == 0 || escape == "" || text == "" {
		return text
	}
	if !opensWithASigil(chars, text) || !leadsWithAttachmentRef(chatID, text) {
		return text
	}
	return escape + text
}

// Strip is Guard's inverse, for the way back: a user_prompt event reports the
// text the CLI ACTUALLY received, escape and all, and that report is what
// becomes the durable ledger turn (see turn.go, which restores the attachment
// path on the same line). Without this the person's own message would be
// stored, forever, one byte longer than they wrote it.
//
// Removes at most one escape, and only in front of exactly what Guard would
// have put it in front of.
func Strip(chars []string, escape, chatID, text string) string {
	if len(chars) == 0 || escape == "" || !strings.HasPrefix(text, escape) {
		return text
	}
	rest := strings.TrimPrefix(text, escape)
	if !opensWithASigil(chars, rest) || !leadsWithAttachmentRef(chatID, rest) {
		return text
	}
	return rest
}
