package render

const (
	AllThreadMessages     = 0
	MaxThreadMessages     = 4
	MaxMessageBodyChars   = 200
	MaxTurnBodyChars      = 800
	MaxScopeRangesPerFile = 6
	MaxScopeRanges        = 300
)

type ChatTurn struct {
	Speaker string
	Body    string
}
