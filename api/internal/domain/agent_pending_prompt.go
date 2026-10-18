package domain

// PendingPrompt is a chat's oldest prompt the provider has not acknowledged yet
// — recovered so a client whose own copy of the text was lost (an idle tab, a
// crash, cleared local storage) can show it back to the user instead of losing
// it outright.
type PendingPrompt struct {
	Text  string
	State string
	// RequestID is the client request id the prompt was recorded under — not
	// freshly minted, so a recovered row matches the frame that later settles it
	// and a resend of it is answered, never delivered twice.
	RequestID string
}
