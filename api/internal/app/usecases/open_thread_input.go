package usecases

import "github.com/char2cs/crowbar/api/internal/domain"

// OpenThreadInput carries a new review thread's anchor + first message (09 §3).
type OpenThreadInput struct {
	WsID       string
	FilePath   string
	LineNumber int
	Side       domain.ReviewSide
	Body       string
}
