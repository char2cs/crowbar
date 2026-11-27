package model

import "image/color"

type TerminalModel interface {
	Write(
		p []byte,
	)

	Resize(
		cols int,
		rows int,
	)

	OnForegroundReset()

	Title() string

	Cols() int

	Rows() int

	HeaderState() (cols, rows int, alt bool, scrollbackLines int)

	ModelBytes() int64

	SetResponseSink(sink func(p []byte))

	Close()
}

type Serializer interface {
	Serialize(
		m TerminalModel,
	) []byte
}

type ThemeAware interface {
	SetDefaultColors(bg, fg color.Color)
	ThemeNotifyEnabled() bool
}

type ScreenReader interface {
	ScreenText() string
}

type ScreenState struct {
	Text           string
	CursorRow      int
	CursorCol      int
	Faint          [][]bool
	BracketedPaste bool
}

type ScreenStateReader interface {
	ScreenState() ScreenState
}

type ModelHealth interface {
	Degraded() bool

	ParsePanics() int
}

func New(
	cols int,
	rows int,
	scrollbackLines int,
) (TerminalModel, Serializer) {
	return newVTModel(cols, rows, scrollbackLines), vtSerializer{}
}
