package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

const (
	defaultSearchLimit = 200
	maxSearchLimit     = 1000
)

type searchResponse struct {
	Hits      []gitdomain.SearchHit `json:"hits"`
	Truncated bool                  `json:"truncated"`
}

func (h *Handlers) SearchDiff(
	ctx *gin.Context,
) {
	opts, err := searchOpts(ctx)
	if err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	hits, truncated, err := h.reviewUsecase.SearchDiff(
		ctx.Request.Context(),
		h.workspaceID(ctx),
		scopeCommit(ctx),
		ctx.Query("q"),
		opts,
	)
	if err != nil {
		libs.Fail(ctx, err)
		return
	}
	if hits == nil {
		hits = []gitdomain.SearchHit{}
	}
	libs.WriteQueryOK(ctx, searchResponse{Hits: hits, Truncated: truncated})
}

func searchOpts(
	ctx *gin.Context,
) (gitdomain.SearchOpts, error) {
	useRegex, err := searchFlag(ctx.Query("regex"), "regex")
	if err != nil {
		return gitdomain.SearchOpts{}, err
	}
	caseSensitive, err := searchFlag(ctx.Query("case"), "case")
	if err != nil {
		return gitdomain.SearchOpts{}, err
	}
	limit, err := searchLimit(ctx.Query("limit"))
	if err != nil {
		return gitdomain.SearchOpts{}, err
	}
	return gitdomain.SearchOpts{
		Regex:         useRegex,
		CaseSensitive: caseSensitive,
		Limit:         limit,
	}, nil
}

func searchFlag(
	raw string,
	name string,
) (bool, error) {
	if raw == "" {
		return false, nil
	}
	on, err := strconv.ParseBool(raw)
	if err != nil {
		return false, errors.New(name + " query parameter must be true or false")
	}
	return on, nil
}

func searchLimit(
	raw string,
) (int, error) {
	if raw == "" {
		return defaultSearchLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit query parameter must be an integer")
	}
	return clampSearchLimit(n), nil
}

func clampSearchLimit(
	n int,
) int {
	if n <= 0 {
		return defaultSearchLimit
	}
	return min(n, maxSearchLimit)
}
