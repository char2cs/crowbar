package handlers

import "github.com/gin-gonic/gin"

const ScopeCommitParam = "sha"

func scopeCommit(ctx *gin.Context) string {
	return ctx.Query(ScopeCommitParam)
}
