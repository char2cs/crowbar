package promptsigil_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
)

// What claude.yaml and codex.yaml actually declare.
var (
	sigils = []string{"!"}
	escape = " "
)

const (
	chatID = "chat-1"
	image  = "![screenshot.png](chats/chat-1/attachments/ab12-screenshot.png)"
)

// REGRESSION. An image attached BEFORE anything was typed serializes to
// `![alt](chats/<id>/attachments/<file>)`, whose own first character is `!` —
// which codex-cli reads as "run the rest as a shell command". Measured on
// 0.149.1 against the exact argv Crowbar dispatches: the prompt never reached
// the model, codex ran `[screenshot.png](/…/shot.png)`, and because no prompt
// event ever fired the message did not become a ledger turn either — it
// vanished. Nobody typed that `!`; markdown's image syntax did.
func TestRegression_AttachmentSentFirstNeverOpensTheCLIsShellMode(t *testing.T) {
	out := promptsigil.Guard(sigils, escape, chatID, image)

	assert.NotEqual(t, "!", out[:1],
		"an attachment sent first must never leave `!` as the message's first character")
	assert.Equal(t, " "+image, out,
		"and the attachment itself must be handed over unchanged behind the escape")
}

// The other half of the same regression: the sigil guarded is only ever
// Crowbar's own, so shell mode a PERSON asked for keeps working byte-for-byte
// — as does the `/compact` Crowbar sends down this very path as prompt text.
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

// Applied twice — a turn re-dispatched into a fresh CLI, say — the escape must
// not stack.
func TestGuard_IsIdempotent(t *testing.T) {
	once := promptsigil.Guard(sigils, escape, chatID, image)
	assert.Equal(t, once, promptsigil.Guard(sigils, escape, chatID, once))
}

// The way back: a user_prompt event reports the text the CLI actually
// received, escape and all, and that report becomes the durable ledger turn.
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
		"  " + image, // two spaces: only one is ever added, so this is the person's
		"",
	} {
		assert.Equal(t, text, promptsigil.Strip(sigils, escape, chatID, text), "%q", text)
	}
}

func TestStrip_IsANoOpForAProviderDeclaringNoSigils(t *testing.T) {
	assert.Equal(t, " "+image, promptsigil.Strip(nil, "", chatID, " "+image))
}
