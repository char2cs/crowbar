package promptsigil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
)

var (
	sigils = []string{"!"}
	escape = " "
)

const (
	chatID = "chat-1"
	image  = "![screenshot.png](chats/chat-1/attachments/ab12-screenshot.png)"
)

func TestRegression_AttachmentSentFirstNeverOpensTheCLIsShellMode(t *testing.T) {
	out := promptsigil.Guard(sigils, escape, chatID, image)

	assert.NotEqual(t, "!", out[:1],
		"an attachment sent first must never leave `!` as the message's first character")
	assert.Equal(t, " "+image, out,
		"and the attachment itself must be handed over unchanged behind the escape")
}

func TestRegression_ADeliberatelyTypedLeadingSigilIsNeverEscaped(t *testing.T) {
	for _, text := range []string{
		"!ls -la",
		"!echo hi " + image,
		"!",
		"/compact",
		"![screenshot.png](https://example.com/shot.png)",
		"![screenshot.png](chats/OTHER-CHAT/attachments/ab12-screenshot.png)",
		"look at " + image,
		"",
	} {
		assert.Equal(t, text, promptsigil.Guard(sigils, escape, chatID, text),
			"%q is not Crowbar's own leading attachment reference and must pass through untouched", text)
	}
}

func TestGuard_IsANoOpForAProviderDeclaringNoSigils(t *testing.T) {
	assert.Equal(t, image, promptsigil.Guard(nil, "", chatID, image))
}

func TestGuard_LeavesTheFileLinkFormAloneBecauseItOpensWithNoSigil(t *testing.T) {
	text := "[notes.pdf](chats/chat-1/attachments/ab12-notes.pdf)"
	assert.Equal(t, text, promptsigil.Guard(sigils, escape, chatID, text))
}

func TestGuard_IsIdempotent(t *testing.T) {
	once := promptsigil.Guard(sigils, escape, chatID, image)
	assert.Equal(t, once, promptsigil.Guard(sigils, escape, chatID, once))
}

func TestStrip_TakesBackExactlyWhatGuardAdded(t *testing.T) {
	assert.Equal(t, image, promptsigil.Strip(sigils, escape, chatID, " "+image))
}

func TestStrip_LeavesAnythingGuardWouldNotHaveTouched(t *testing.T) {
	for _, text := range []string{
		image,
		" hello world",
		" !ls -la",
		" [notes.pdf](chats/chat-1/attachments/ab12-notes.pdf)",
		" ![screenshot.png](chats/OTHER-CHAT/attachments/ab12-screenshot.png)",
		"  " + image,
		"",
	} {
		assert.Equal(t, text, promptsigil.Strip(sigils, escape, chatID, text), "%q", text)
	}
}

func TestStrip_IsANoOpForAProviderDeclaringNoSigils(t *testing.T) {
	assert.Equal(t, " "+image, promptsigil.Strip(nil, "", chatID, " "+image))
}
