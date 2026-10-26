package models

type TerminalPrompt struct {
	Kind string

	Needle string

	Answers []TerminalPromptAnswer
}

type TerminalPromptAnswer struct {
	ID string

	Label string

	Keys string
}
