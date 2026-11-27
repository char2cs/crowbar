package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeRepos struct {
	repo    *domain.Repository
	err     error
	gotID   string
	lookups int
}

func (f *fakeRepos) FindByKey(
	_ context.Context,
	id string,
) (*domain.Repository, error) {
	f.lookups++
	f.gotID = id
	return f.repo, f.err
}

func importableRepo() *fakeRepos {
	return &fakeRepos{repo: &domain.Repository{
		ID:        "r1",
		ProjectID: "p1",
		Path:      "/repos/acme",
		RemoteURL: "git@github.com:acme/acme.git",
	}}
}

func newImportingHandlers(
	tree *fakeChatTree,
	repos handlers.Repos,
) *handlers.Handlers {
	return newChatHandlersWith(&fakeAgentUsecase{}, tree).WithRepos(repos)
}

func repoScopedCreate(
	t *testing.T,
	h *handlers.Handlers,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	ctx, rec := newTestContext(t, http.MethodPost, "/v0/projects/p1/repos/r1/chats", []byte(body))
	ctx.Params = gin.Params{{Key: "projectId", Value: "p1"}, {Key: "repoId", Value: "r1"}}
	h.Create(ctx)
	return rec
}

func TestCreate_ImportBodyProducesAWorktreeImportCreate(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	repos := importableRepo()
	h := newImportingHandlers(tree, repos)

	rec := repoScopedCreate(t, h,
		`{"provider":"vendor-a","import":{"branch":"feature/x"}}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, 1, tree.createChatCalls)
	assert.Equal(t, agentusecase.WorktreeImport, tree.gotWorktree.Mode,
		"an import body must not flatten to a plain chat")
	assert.Equal(t, "feature/x", tree.gotWorktree.Import.Branch)
	assert.Equal(t, "vendor-a", tree.gotCreate2.ProviderID)
	assert.Empty(t, tree.gotCreate2.WorkspaceID,
		"an import mints its own workspace; it never attaches to one named up front")
}

func TestCreate_ImportTakesItsRepoFactsFromThePathsRepo(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	repos := importableRepo()
	h := newImportingHandlers(tree, repos)

	rec := repoScopedCreate(t, h, `{"import":{"branch":"feature/x"}}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "r1", repos.gotID, "the repo resolved is the one in the URL")
	spec := tree.gotWorktree.Import
	assert.Equal(t, "r1", spec.RepoID)
	assert.Equal(t, "p1", spec.ProjectID)
	assert.Equal(t, "/repos/acme", spec.RepoPath)
	assert.Equal(t, "git@github.com:acme/acme.git", spec.RemoteURL)
}

func TestCreate_ImportRemoteOverridesTheReposOwn(
	t *testing.T,
) {
	for name, tc := range map[string]struct {
		body string
		want string
	}{
		"omitted takes the repo's remote": {
			body: `{"import":{"branch":"feature/x"}}`,
			want: "git@github.com:acme/acme.git",
		},
		"an explicit remote wins": {
			body: `{"import":{"branch":"feature/x","remote":"git@example.com:fork.git"}}`,
			want: "git@example.com:fork.git",
		},
		"an explicit empty remote means local-only": {
			body: `{"import":{"branch":"feature/x","remote":""}}`,
			want: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
			h := newImportingHandlers(tree, importableRepo())

			rec := repoScopedCreate(t, h, tc.body)

			require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
			assert.Equal(t, tc.want, tree.gotWorktree.Import.RemoteURL)
		})
	}
}

func TestCreate_ImportStillPlacesTheChatUnderItsParent(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newImportingHandlers(tree, importableRepo())

	rec := repoScopedCreate(t, h,
		`{"parentId":"parent-chat","import":{"branch":"feature/x"}}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "parent-chat", tree.gotCreate2.ParentID)
	assert.Empty(t, tree.gotWorktree.Import.ParentWorkspaceID)
	assert.Empty(t, tree.gotWorktree.Import.ParentBranch)
}

func TestCreate_ImportWithoutABranchIsRefused(
	t *testing.T,
) {
	for name, body := range map[string]string{
		"absent":     `{"import":{}}`,
		"empty":      `{"import":{"branch":""}}`,
		"whitespace": `{"import":{"branch":"   "}}`,
	} {
		t.Run(name, func(t *testing.T) {
			tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
			h := newImportingHandlers(tree, importableRepo())

			rec := repoScopedCreate(t, h, body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Zero(t, tree.createChatCalls, "nothing is created for a branchless import")
		})
	}
}

func TestCreate_ForkAndImportTogetherAreRefused(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newImportingHandlers(tree, importableRepo())

	rec := repoScopedCreate(t, h,
		`{"ownWorktree":true,"import":{"branch":"feature/x"}}`)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Zero(t, tree.createChatCalls)
}

func TestCreate_ImportAlongsideAWorkspaceIsRefused(
	t *testing.T,
) {
	t.Run("named in the body", func(t *testing.T) {
		tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
		h := newImportingHandlers(tree, importableRepo())

		rec := repoScopedCreate(t, h,
			`{"workspaceId":"ws-9","import":{"branch":"feature/x"}}`)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Zero(t, tree.createChatCalls)
	})

	t.Run("injected by the home mount", func(t *testing.T) {
		tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
		h := newImportingHandlers(tree, importableRepo())

		ctx, rec := newTestContext(t, http.MethodPost,
			"/v0/projects/p1/repos/r1/workspaces/ws-1/chats",
			[]byte(`{"import":{"branch":"feature/x"}}`))
		ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}
		h.Create(ctx)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Zero(t, tree.createChatCalls)
	})
}

func TestCreate_ImportForAnUnknownRepoIs404(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newImportingHandlers(tree, &fakeRepos{})

	rec := repoScopedCreate(t, h, `{"import":{"branch":"feature/x"}}`)

	require.Equal(t, http.StatusNotFound, rec.Code)
	assert.Zero(t, tree.createChatCalls)
}

func TestCreate_ImportSurfacesARepoLookupFailure(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	h := newImportingHandlers(tree, &fakeRepos{err: errors.New("the store is down")})

	rec := repoScopedCreate(t, h, `{"import":{"branch":"feature/x"}}`)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Zero(t, tree.createChatCalls)
}

func TestCreate_WithoutARepoStore(
	t *testing.T,
) {
	t.Run("an import is refused", func(t *testing.T) {
		tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
		h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

		rec := repoScopedCreate(t, h, `{"import":{"branch":"feature/x"}}`)

		require.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Zero(t, tree.createChatCalls)
	})

	t.Run("an ordinary create still works", func(t *testing.T) {
		tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
		h := newChatHandlersWith(&fakeAgentUsecase{}, tree)

		rec := repoScopedCreate(t, h, `{"provider":"vendor-a","ownWorktree":true,"parentId":"c-owner"}`)

		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Equal(t, agentusecase.WorktreeFork, tree.gotWorktree.Mode)
	})
}

func TestCreate_WithoutAnImportReadsNoRepo(
	t *testing.T,
) {
	tree := &fakeChatTree{placed: domain.Chat{ID: "chat-1"}}
	repos := importableRepo()
	h := newImportingHandlers(tree, repos)

	rec := repoScopedCreate(t, h, `{"provider":"vendor-a"}`)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Zero(t, repos.lookups)
	assert.Equal(t, agentusecase.WorktreeNone, tree.gotWorktree.Mode)
}
