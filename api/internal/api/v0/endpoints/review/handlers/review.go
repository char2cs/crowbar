package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/char2cs/crowbar/api/internal/api/libs"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// reviewErrorStatus maps a usecase error to an HTTP status: a genuine
// not-found entity yields 404, a request the usecase refused as malformed (an
// empty patch path, an uncompilable search pattern) yields 400, and every other
// (internal) failure yields 500, so a real git/subprocess error is never masked
// as a 404 and a user's half-typed regex is never masked as a daemon crash.
func reviewErrorStatus(
	err error,
) int {
	if errors.Is(err, apperr.ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, apperr.ErrInvalidArgument) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

// Get handles GET /v0/workspaces/:wsId/review, returning the composite
// branch-review read model for the workspace.
//
// The response goes through BranchReviewDTOFrom rather than the domain object.
// That is not cosmetic: a FileDiff for a BINARY file carries a nil Lines slice,
// which marshals to JSON `null`, and the review pane reads `diff.lines.length`
// unguarded — so a single binary file anywhere in a branch took the whole pane
// down with "null is not an object". The DTO normalises those slices to `[]`,
// and did so all along; this endpoint simply was not using it. It was reachable
// only from its own tests, one of which
// (TestBranchReviewDTOFromEmptySlicesAreNonNil) asserts exactly the property
// whose absence caused the crash.
func (h *Handlers) Get(
	ctx *gin.Context,
) {
	review, err := h.reviewUsecase.Get(ctx.Request.Context(), h.workspaceID(ctx))
	if err != nil {
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, dto.BranchReviewDTOFrom(review))
}

// SetMergeStrategy handles PATCH /v0/workspaces/:wsId/review, updating the
// merge strategy for the workspace.
func (h *Handlers) SetMergeStrategy(
	ctx *gin.Context,
) {
	var body struct {
		MergeStrategy gitdomain.MergeStrategy `json:"mergeStrategy"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		libs.WriteErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.reviewUsecase.SetMergeStrategy(ctx.Request.Context(), h.workspaceID(ctx), body.MergeStrategy); err != nil {
		libs.WriteErr(ctx, reviewErrorStatus(err), err.Error())
		return
	}
	libs.WriteQueryOK(ctx, gin.H{"mergeStrategy": body.MergeStrategy})
}
