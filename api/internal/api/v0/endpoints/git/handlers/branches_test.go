package handlers_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

func TestBranchesSuccess(
	t *testing.T,
) {
	ahead := 3
	when := time.Unix(0, 0).UTC()
	git := &fakeGit{
		branches: []gitdomain.Branch{
			{Name: "main", IsCurrent: true, Ahead: &ahead, LastCommitDate: &when},
		},
	}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/branches")

	assert.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []struct {
			Name           string  `json:"name"`
			IsCurrent      bool    `json:"isCurrent"`
			Ahead          *int    `json:"ahead"`
			LastCommitDate *string `json:"lastCommitDate"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "main", body.Data[0].Name)
	assert.True(t, body.Data[0].IsCurrent)
	require.NotNil(t, body.Data[0].Ahead)
	assert.Equal(t, 3, *body.Data[0].Ahead)
	require.NotNil(t, body.Data[0].LastCommitDate)
	assert.Equal(t, "1970-01-01T00:00:00Z", *body.Data[0].LastCommitDate)
}

func TestBranchesEmptyNonNil(
	t *testing.T,
) {
	rec := do(newRouter(&fakeGit{}), "/v0/workspaces/w1/git/branches")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"data":[]`)
}

func TestBranchesError(
	t *testing.T,
) {
	git := &fakeGit{branchesErr: errors.New("boom")}
	rec := do(newRouter(git), "/v0/workspaces/w1/git/branches")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
