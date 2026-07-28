package handlers

import "github.com/gin-gonic/gin"

// ScopeCommitParam is the query parameter that narrows a windowed review read
// from the workspace's branch diff to a single commit.
const ScopeCommitParam = "sha"

// scopeCommit reads the commit a windowed read is scoped to, or "" for the
// branch diff.
//
// Absent means the branch — the default and the surface's reason for existing —
// so every existing caller keeps its behaviour without sending anything new.
// The value is NOT validated here: the usecase owns that (resolveScopeRef
// rejects anything that is not a hex object name), because it is the layer that
// turns the string into a git argument and the guard belongs with the risk.
func scopeCommit(ctx *gin.Context) string {
	return ctx.Query(ScopeCommitParam)
}
