package tools

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools/internal/render"
)

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
