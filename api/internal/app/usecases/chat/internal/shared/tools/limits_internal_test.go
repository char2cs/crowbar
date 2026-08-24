package tools

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
)

// The result caps, exposed so the bounding tests build fixtures FROM the
// production numbers rather than from copies of them. A test that hardcoded 20
// would keep passing against a cap someone had since changed to 200, which is
// the one way a bounding test can go quiet without anyone noticing.
const (
	DefaultThreadPageForTest     = defaultThreadPage
	MaxThreadPageForTest         = maxThreadPage
	MaxThreadMessagesForTest     = render.MaxThreadMessages
	DefaultChatLogTurnsForTest   = defaultChatLogTurns
	MaxChatLogTurnsForTest       = maxChatLogTurns
	DefaultScopeFilesForTest     = defaultScopeFiles
	MaxScopeFilesForTest         = maxScopeFiles
	MaxScopeRangesPerFileForTest = render.MaxScopeRangesPerFile
	MaxScopeRangesForTest        = render.MaxScopeRanges
	MaxMessageBodyCharsForTest   = render.MaxMessageBodyChars
	MaxTurnBodyCharsForTest      = render.MaxTurnBodyChars
	MaxWrittenBodyCharsForTest   = render.MaxBodyChars
)
