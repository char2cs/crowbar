package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

func createdBody(t *testing.T, body []byte) (id, workspaceID string) {
	t.Helper()
	var env struct {
		Data struct {
			ID          string `json:"id"`
			WorkspaceID string `json:"workspaceId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	return env.Data.ID, env.Data.WorkspaceID
}

func TestCreate_UsesTheClientChatID(t *testing.T) {
	id := uuid.NewString()
	tree := &fakeChatTree{placed: domain.Chat{ID: id}}

	rec := repoScopedCreate(t, newImportingHandlers(tree, importableRepo()),
		`{"id":"`+id+`","provider":"claude","workspaceId":"ws-1"}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, id, tree.gotCreate2.ID)
	got, _ := createdBody(t, rec.Body.Bytes())
	assert.Equal(t, id, got)
}

func TestCreate_AnIDThatIsNotAUUIDIsRefused(t *testing.T) {
	tree := &fakeChatTree{}

	rec := repoScopedCreate(t, newImportingHandlers(tree, importableRepo()),
		`{"id":"row-1","provider":"claude","workspaceId":"ws-1"}`)

	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Zero(t, tree.createChatCalls, "nothing is created under a malformed id")
}

func TestCreate_ATakenIDIsAnIDConflict(t *testing.T) {
	tree := &fakeChatTree{err: &problem.Error{Code: problem.IDConflict, Message: "chat exists"}}

	rec := repoScopedCreate(t, newImportingHandlers(tree, importableRepo()),
		`{"id":"`+uuid.NewString()+`","provider":"claude","workspaceId":"ws-1"}`)

	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), string(problem.IDConflict))
}

func TestCreate_AnOwnWorktreeCreateAnswersItsWorkspace(t *testing.T) {
	id := uuid.NewString()
	tree := &fakeChatTree{placed: domain.Chat{ID: id}, createdWorkspace: "ws-new"}

	rec := repoScopedCreate(t, newImportingHandlers(tree, importableRepo()),
		`{"id":"`+id+`","provider":"claude","ownWorktree":true,"branch":"feat/x","parentId":"c-owner"}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	gotID, gotWorkspace := createdBody(t, rec.Body.Bytes())
	assert.Equal(t, id, gotID)
	assert.Equal(t, "ws-new", gotWorkspace)
}

func TestCreate_AnOwnWorktreeCreateWithNoParentIsAnInvalidArgument(t *testing.T) {
	tree := &fakeChatTree{createdWorkspace: "ws-new"}

	rec := repoScopedCreate(t, newImportingHandlers(tree, importableRepo()),
		`{"id":"`+uuid.NewString()+`","provider":"claude","ownWorktree":true}`)

	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), string(problem.InvalidArgument))
	assert.Zero(t, tree.createChatCalls, "nothing is created without a parent to fork from")
}
