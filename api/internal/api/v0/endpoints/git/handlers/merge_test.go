package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMergeAndRebase_AnswerTheirOperation(t *testing.T) {
	r := newRouter()

	merge := accepted(t, do(r, http.MethodPost, ws+"/merge", map[string]any{"branch": "main"}))
	assert.Equal(t, domain.OpGitMerge, merge.Kind)
	assert.Equal(t, "Merge main", merge.Label)

	rebase := accepted(t, do(r, http.MethodPost, ws+"/rebase", map[string]any{"branch": "main"}))
	assert.Equal(t, domain.OpGitRebase, rebase.Kind)
	assert.Equal(t, "Rebase onto main", rebase.Label)
}

// A merge or rebase missing its branch is refused before any operation starts.
func TestMergeAndRebase_ValidationFailsSync_4xx(
	t *testing.T,
) {
	r := newRouter()

	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/merge", map[string]any{}).Code)
	assert.Equal(t, http.StatusBadRequest, do(r, http.MethodPost, ws+"/rebase", map[string]any{}).Code)
}
