package libs

import (
	"errors"
	"io/fs"
	"net/http"

	asynxmodels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	enginesearch "github.com/char2cs/crowbar/api/internal/engine/search"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// StatusAndMessage maps a known domain, app, or engine sentinel error to the
// HTTP status code a handler should emit, alongside the message to surface in
// the error envelope. It uses errors.Is, so wrapped errors map correctly.
//
// The mapping is open for extension: new categories are added by appending
// guard clauses. The categories are:
//
//   - 404 Not Found      — apperr.ErrNotFound, engineterminal.ErrSessionNotFound,
//     asynxmodels.ErrNotFound (the asynx aggregate-not-found sentinel surfaced
//     by the aggregate usecases), and fs.ErrNotExist (the raw filesystem
//     not-found error wrapped up from the fs engine).
//   - 400 Bad Request    — enginesearch.ErrBadPattern,
//     enginesearch.ErrPathOutsideWorkspace.
//   - 409 Conflict        — enginesearch.ErrLocked and the worktree lock /
//     non-leaf sentinels (ErrParentLocked, ErrNewParentLocked,
//     ErrRebaseNonLeaf, ErrChildHasChildren).
//   - 500 Internal Error  — any other (or nil) error.
//
// A 503 "engine unavailable" category is intentionally absent: the v0 handlers
// guard nil engines before any usecase runs (the requireXEngine helpers), so
// unavailability never reaches this mapper as an error value.
func StatusAndMessage(
	err error,
) (int, string) {
	if err == nil {
		return http.StatusInternalServerError, "internal error"
	}

	if errors.Is(err, apperr.ErrNotFound) ||
		errors.Is(err, engineterminal.ErrSessionNotFound) ||
		errors.Is(err, asynxmodels.ErrNotFound) ||
		errors.Is(err, fs.ErrNotExist) {
		return http.StatusNotFound, err.Error()
	}

	if errors.Is(err, enginesearch.ErrBadPattern) ||
		errors.Is(err, enginesearch.ErrPathOutsideWorkspace) {
		return http.StatusBadRequest, err.Error()
	}

	if isConflict(err) {
		return http.StatusConflict, err.Error()
	}

	return http.StatusInternalServerError, err.Error()
}

// isConflict reports whether err is one of the lock or non-leaf conflict
// sentinels that map to HTTP 409.
func isConflict(
	err error,
) bool {
	if errors.Is(err, enginesearch.ErrLocked) ||
		errors.Is(err, worktree.ErrParentLocked) ||
		errors.Is(err, worktree.ErrNewParentLocked) {
		return true
	}

	if errors.Is(err, worktree.ErrRebaseNonLeaf) ||
		errors.Is(err, worktree.ErrChildHasChildren) {
		return true
	}

	return false
}
