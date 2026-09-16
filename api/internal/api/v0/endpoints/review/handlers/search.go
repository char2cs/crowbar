package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// defaultSearchLimit and maxSearchLimit bound every search this endpoint runs.
//
// gitdomain.SearchOpts treats Limit <= 0 as unlimited, which is safe only for a
// caller that knows its query is narrow. No HTTP caller does: a one-letter query
// over a million-line diff would collect a hit per line. The endpoint therefore
// never exposes that mode — every value out of range lands inside the bounds
// instead of disabling them.
const (
	defaultSearchLimit = 200
	maxSearchLimit     = 1000
)

// searchResponse is the data payload of GET /review/search.
type searchResponse struct {
	Hits      []gitdomain.SearchHit `json:"hits"`
	Truncated bool                  `json:"truncated"`
}

// SearchDiff handles GET /v0/chats/:chatId/review/search?q=&regex=&case=&limit=,
// returning the file and line number of every match in the branch diff's
// content plus whether the limit cut the results short.
//
// It is find-in-diff moved server-side: the client only ever holds a window of
// the patch, so it has nothing left to search locally. An empty q is the state
// a find box opens in and answers an empty result rather than an error or a
// whole-diff scan.
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
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
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

// searchFlag reads a boolean query parameter. An absent one is false — the
// forgiving literal, case-insensitive search a find box wants — while an
// unparseable one is a 400, because silently reading a typo as "false" would
// answer a different search than the one asked for.
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
