// Package promptsigil escapes a message's leading CLI sigil (`!` opens a shell) so an attachment
// written as `![alt](chats/<chatID>/attachments/<file>)` reaches the model, and undoes it on the
// way back. The sigils and their escape come from presentation.prompt_submit.leading_sigils.
package promptsigil

import (
	"regexp"
	"slices"
	"strings"
)

// leadingAttachmentPattern matches a message opening with a durable attachment reference, image form
// `![alt](…)` or link form `[name](…)`. Neither the chatID nor the filename may contain "/", so neither
// can walk out of the store.
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

// Guard prefixes escape to text only when its leading sigil is one Crowbar wrote (its own leading
// attachment reference); anything else passes through byte-identical, so a typed `!` or a `/compact`
// keeps its meaning. Idempotent: a guarded text no longer opens with a sigil.
func Guard(chars []string, escape, chatID, text string) string {
	if len(chars) == 0 || escape == "" || text == "" {
		return text
	}
	if !opensWithASigil(chars, text) || !leadsWithAttachmentRef(chatID, text) {
		return text
	}
	return escape + text
}

// Strip is Guard's inverse: a user_prompt event reports the text the CLI received, escape included, and
// that becomes the durable ledger turn. It removes at most one escape, and only where Guard would have
// added it.
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
