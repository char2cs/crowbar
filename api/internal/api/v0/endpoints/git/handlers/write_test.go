package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCommit_StillReturns200 asserts the fast write path is unchanged: commit
// validates and runs synchronously, returning 200.
func TestCommit_StillReturns200(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodPost, ws+"/commit", map[string]any{"subject": "feat: add"})
	assert.Equal(t, http.StatusOK, rec.Code)
}
