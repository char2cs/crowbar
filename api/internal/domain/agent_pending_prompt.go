package domain

// PendingPrompt is the most recent prompt submission a chat's journal has not
// confirmed the provider accepted — recovered so a client whose own copy of
// the text was lost (an idle tab, a crash, cleared local storage) can show it
// back to the user instead of losing it outright.
type PendingPrompt struct {
	Text  string
	State string
}
