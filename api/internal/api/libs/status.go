package libs

import (
	"errors"
	"io/fs"
	"net/http"

	asynxmodels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agentchatfolder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/folder"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/engine/fs/safepath"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
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
//     by the aggregate usecases), fs.ErrNotExist (the raw filesystem
//     not-found error wrapped up from the fs engine),
//     project.ErrFolderNotFound (a project import targeting a path that does
//     not exist on disk), enginegit.ErrBranchNotFound (a branch or
//     revision operand git could not resolve), agentchat.ErrNotFound (an
//     agent chat/segment id the agentic-chat repo has no row for), and
//     agentrunner.ErrNotFound (a runner id — a `--segment` value — with no
//     live row, either never spawned or already exited).
//   - 400 Bad Request    — folder.ErrFolderNameRequired and
//     agentchatfolder.ErrNameRequired (a folder create or rename with a blank
//     name, in the sidebar and the Chats panel respectively),
//     enginesearch.ErrBadPattern,
//     enginesearch.ErrPathOutsideWorkspace, safepath.ErrPathEscapesWorkspace
//     (a workspace-relative fs path that is absolute or traverses outside the
//     workspace root via ".." or a symlink — the fs engine containment guard),
//     apperr.ErrInvalidArgument (an unsafe/invalid git operand or reset mode
//     rejected at the usecase boundary before it can reach the git engine —
//     see the git write validator), fs.ErrInvalid (an invalid fs-engine
//     operand, e.g. a copy destination inside its own source directory), and
//     enginegit.ErrNoRemote (no remote configured or the remote URL is
//     unreachable).
//   - 413 Request Entity Too Large — safepath.ErrFileTooLarge (a file read was
//     rejected because the file exceeds the 25 MiB cap; hardening R16).
//   - 403 Forbidden       — enginegit.ErrAuthFailed (remote rejected the
//     supplied credentials on push/pull/fetch; a forbidden-style auth failure,
//     not a transport outage).
//   - 424 Failed Dependency — engineterminal.ErrCommandNotFound (the vendor CLI
//     a spawn needs — claude, codex — is not installed on this machine or is not
//     executable) and apperr.ErrFailedDependency (a CLI that IS installed but
//     failed to come up — above all one that exited during startup, so the chat
//     never got the live TUI it asked for). A broken dependency the USER can fix,
//     not a server fault.
//   - 409 Conflict        — apperr.ErrLocked (a write against a locked,
//     provider-protected workspace; 04 §5, 05 §3/§4), fs.ErrExist (an fs
//     mutation whose destination already exists, e.g. a copy landing on an
//     existing path), enginesearch.ErrLocked,
//     project.ErrRepoAlreadyImported (a repo folder a different project has
//     already imported — one folder belongs to exactly one project),
//     the worktree lock / non-leaf sentinels (ErrParentLocked,
//     ErrWorkspaceLocked, ErrRebaseNonLeaf,
//     ErrChildHasChildren), the sidebar-placement sentinels
//     (folder.ErrFolderCycle, folder.ErrFolderCrossRepo,
//     folder.ErrForkChainSplit — a move that would make a row unreachable, cross
//     a repo boundary, or split a fork chain), and the git
//     engine's classified conflict sentinels (ErrConflict, ErrDirtyTree,
//     ErrRejectedNonFastForward, ErrNothingToCommit, ErrStaleHunk,
//     ErrHasChildren, ErrBranchAlreadyExists, ErrNonFastForward).
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

	if isNotFound(err) {
		return http.StatusNotFound, err.Error()
	}

	if isBadRequest(err) {
		return http.StatusBadRequest, err.Error()
	}

	if errors.Is(err, safepath.ErrFileTooLarge) {
		return http.StatusRequestEntityTooLarge, err.Error()
	}

	if errors.Is(err, enginegit.ErrAuthFailed) {
		return http.StatusForbidden, err.Error()
	}

	// agenttools.ErrUnauthorized is a runner callback that could not prove it is
	// the runner it names: a bad or absent per-boot token, or an id with no runner
	// behind it. 403 rather than 401 because there is no challenge to issue — the
	// credential is minted at spawn and there is nothing the caller can be asked
	// for — and rather than 500 because the daemon is perfectly healthy and the
	// request is simply not authorised.
	if errors.Is(err, agenttools.ErrUnauthorized) {
		return http.StatusForbidden, err.Error()
	}

	// engineterminal.ErrCommandNotFound is a MISSING DEPENDENCY on the user's machine:
	// the vendor CLI a spawn needs (claude, codex) is not installed or not executable.
	// The request was well-formed and the server is healthy, so it is neither a
	// client-got-it-wrong 4xx nor a 500 — it is 424 Failed Dependency. It is broken out
	// of the 500 bucket precisely because it is the ONE spawn failure the user can act
	// on, and it has to reach them as words rather than as a button that does nothing.
	if errors.Is(err, engineterminal.ErrCommandNotFound) ||
		errors.Is(err, apperr.ErrFailedDependency) {
		return http.StatusFailedDependency, err.Error()
	}

	// asynxmodels.ErrValidation is an aggregate state-machine guard rejection (a
	// command Validate said no): the request was well-formed but not applicable to
	// the aggregate's current state, so it is 422 Unprocessable Entity — distinct
	// from the earlier decode/shape 400 (asynx-alignment refactor, spec §3.5).
	if errors.Is(err, asynxmodels.ErrValidation) {
		return http.StatusUnprocessableEntity, err.Error()
	}
	if errors.Is(err, apperr.ErrUnprocessable) {
		return http.StatusUnprocessableEntity, err.Error()
	}
	if errors.Is(err, apperr.ErrTimeout) {
		return http.StatusGatewayTimeout, err.Error()
	}
	if errors.Is(err, apperr.ErrBadGateway) {
		return http.StatusBadGateway, err.Error()
	}

	// apperr.ErrUnavailable is a full asynx shard queue (ErrQueueFull) surfaced by
	// the workspace repo under load: the mutation was not accepted and the client
	// should retry, so it is 503 Service Unavailable.
	if errors.Is(err, apperr.ErrUnavailable) {
		return http.StatusServiceUnavailable, err.Error()
	}

	if isConflict(err) {
		return http.StatusConflict, err.Error()
	}

	return http.StatusInternalServerError, err.Error()
}

// isNotFound reports whether err is one of the sentinels that map to HTTP 404.
func isNotFound(
	err error,
) bool {
	return errors.Is(err, apperr.ErrNotFound) ||
		errors.Is(err, engineterminal.ErrSessionNotFound) ||
		errors.Is(err, asynxmodels.ErrNotFound) ||
		errors.Is(err, fs.ErrNotExist) ||
		errors.Is(err, project.ErrFolderNotFound) ||
		errors.Is(err, enginegit.ErrBranchNotFound) ||
		errors.Is(err, agentchat.ErrNotFound) ||
		errors.Is(err, agentrunner.ErrNotFound)
}

// isBadRequest reports whether err is one of the sentinels that map to HTTP 400.
// fs.ErrInvalid is the fs engine's invalid-operand rejection (e.g. a copy whose
// destination lies inside its own source directory).
func isBadRequest(
	err error,
) bool {
	return errors.Is(err, enginesearch.ErrBadPattern) ||
		errors.Is(err, enginesearch.ErrPathOutsideWorkspace) ||
		errors.Is(err, safepath.ErrPathEscapesWorkspace) ||
		errors.Is(err, apperr.ErrInvalidArgument) ||
		errors.Is(err, fs.ErrInvalid) ||
		errors.Is(err, folder.ErrFolderNameRequired) ||
		errors.Is(err, agentchatfolder.ErrNameRequired) ||
		errors.Is(err, enginegit.ErrNoRemote)
}

// isConflict reports whether err is one of the lock or non-leaf conflict
// sentinels that map to HTTP 409. fs.ErrExist is the fs engine's
// already-exists rejection (e.g. a copy destination that is already on disk).
func isConflict(
	err error,
) bool {
	// asynxmodels.ErrPipelineFailed surfaced after the workspace repo's OCC retries
	// are exhausted is an unrecoverable optimistic-concurrency/version collision →
	// 409 Conflict (asynx-alignment refactor, spec §3.5 ErrPipelineFailed→409).
	if errors.Is(err, asynxmodels.ErrPipelineFailed) {
		return true
	}

	if errors.Is(err, apperr.ErrLocked) ||
		errors.Is(err, apperr.ErrConflict) ||
		errors.Is(err, enginesearch.ErrLocked) ||
		errors.Is(err, fs.ErrExist) ||
		errors.Is(err, worktree.ErrParentLocked) ||
		errors.Is(err, worktree.ErrWorkspaceLocked) ||
		errors.Is(err, worktree.ErrParentUnprovisioned) ||
		errors.Is(err, project.ErrRepoAlreadyImported) {
		return true
	}

	if isPlacementConflict(err) {
		return true
	}

	if errors.Is(err, worktree.ErrRebaseNonLeaf) ||
		errors.Is(err, worktree.ErrChildHasChildren) ||
		errors.Is(err, worktree.ErrBranchWorkspaceExists) ||
		errors.Is(err, worktree.ErrRenameTargetExists) ||
		errors.Is(err, worktree.ErrRenameUnmanagedWorkspace) {
		return true
	}

	return isGitConflict(err)
}

// isPlacementConflict reports whether err is one of the tree-placement sentinels
// that map to HTTP 409: a move that would make a row unreachable from its tree's
// root, cross a repo or workspace boundary, or split a fork chain. Both trees
// are covered — the sidebar's (folder) and the Chats panel's (agentchatfolder) —
// because a refused drag is the same answer to the user either way.
func isPlacementConflict(
	err error,
) bool {
	return errors.Is(err, folder.ErrFolderCycle) ||
		errors.Is(err, folder.ErrFolderCrossRepo) ||
		errors.Is(err, folder.ErrForkChainSplit) ||
		errors.Is(err, agentchatfolder.ErrCycle) ||
		errors.Is(err, agentchatfolder.ErrCrossWorkspace)
}

// isGitConflict reports whether err is one of the git engine's classified
// conflict sentinels that map to HTTP 409.
func isGitConflict(
	err error,
) bool {
	if errors.Is(err, enginegit.ErrConflict) ||
		errors.Is(err, enginegit.ErrDirtyTree) ||
		errors.Is(err, enginegit.ErrRejectedNonFastForward) {
		return true
	}

	if errors.Is(err, enginegit.ErrNothingToCommit) ||
		errors.Is(err, enginegit.ErrStaleHunk) ||
		errors.Is(err, enginegit.ErrHasChildren) {
		return true
	}

	if errors.Is(err, enginegit.ErrBranchAlreadyExists) ||
		errors.Is(err, enginegit.ErrNonFastForward) {
		return true
	}

	return false
}
