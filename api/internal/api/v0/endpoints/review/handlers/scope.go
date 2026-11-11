package handlers

import "github.com/gin-gonic/gin"

// ScopeCommitParam is the query parameter that narrows a windowed review read
// from the workspace's branch diff to a single commit.
const ScopeCommitParam = "sha"

// scopeCommit reads the commit a windowed read is scoped to, or "" for the branch diff. It is not
// validated here: the usecase turns it into a git argument, so its resolveScopeRef owns the guard.
func scopeCommit(ctx *gin.Context) string {
	return ctx.Query(ScopeCommitParam)
}
