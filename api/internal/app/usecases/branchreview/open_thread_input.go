package branchreview

import "github.com/char2cs/crowbar/api/internal/domain"

type OpenThreadInput struct {
	WsID       string
	FilePath   string
	LineNumber int
	Side       domain.ReviewSide
	Body       string
}
