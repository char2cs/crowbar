package promptsigil

import (
	"regexp"
	"slices"
	"strings"
)

var leadingAttachmentPattern = regexp.MustCompile(
	`^!?\[[^\]\n]*\]\(chats/([^/\s)]+)/attachments/[^/)\s]+\)`,
)

func leadsWithAttachmentRef(chatID, text string) bool {
	match := leadingAttachmentPattern.FindStringSubmatch(text)
	return match != nil && match[1] == chatID
}

func opensWithASigil(chars []string, text string) bool {
	return slices.ContainsFunc(chars, func(char string) bool { return strings.HasPrefix(text, char) })
}

func Guard(chars []string, escape, chatID, text string) string {
	if len(chars) == 0 || escape == "" || text == "" {
		return text
	}
	if !opensWithASigil(chars, text) || !leadsWithAttachmentRef(chatID, text) {
		return text
	}
	return escape + text
}

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
