package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	repohandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/repos/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/operation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

var errStore = errors.New("store failure")

// recordingRepoBroadcaster captures each broadcast RepoDTO.
type recordingRepoBroadcaster struct {
	ch chan dto.RepoDTO
}

func newRecordingRepoBroadcaster() *recordingRepoBroadcaster {
	return &recordingRepoBroadcaster{ch: make(chan dto.RepoDTO, 4)}
}

func (b *recordingRepoBroadcaster) push(d dto.RepoDTO) { b.ch <- d }

// next is the next frame already broadcast; read once the operation has
// settled, when its work has returned.
func (b *recordingRepoBroadcaster) next(t *testing.T) dto.RepoDTO {
	t.Helper()
	select {
	case d := <-b.ch:
		return d
	default:
		t.Fatal("no broadcast")
		return dto.RepoDTO{}
	}
}

// assertNoBroadcast is exact once nothing can still write: the request was
// refused, or its operation has settled.
func assertNoBroadcast(t *testing.T, bc *recordingRepoBroadcaster) {
	t.Helper()
	select {
	case d := <-bc.ch:
		t.Fatalf("unexpected broadcast: %+v", d)
	default:
	}
}

// recordedOps is the real operation service over an in-memory store, with
// every saved row on frames.
type recordedOps struct {
	*operation.Service
	frames chan domain.Operation
}

func newRecordedOps(t *testing.T) *recordedOps {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	opStore, err := storesqlite.NewFromDB[domain.Operation, string](db)
	require.NoError(t, err)
	frames := make(chan domain.Operation, 64)
	return &recordedOps{Service: operation.New(opStore, func(op domain.Operation) { frames <- op }, time.Now), frames: frames}
}

// settled decodes the operation a 202 answered with and reads frames until it
// has settled.
func (o *recordedOps) settled(t *testing.T, rec *httptest.ResponseRecorder) domain.Operation {
	t.Helper()
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	var env struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	deadline := time.After(5 * time.Second)
	for {
		select {
		case op := <-o.frames:
			if op.ID == env.Data.Operation.ID && (op.State == domain.OpSucceeded || op.State == domain.OpFailed) {
				return op
			}
		case <-deadline:
			t.Fatalf("operation %s never settled", env.Data.Operation.ID)
		}
	}
}

func statRepoOK(string) (os.FileInfo, error) { return nil, nil }

// branchOutcome is what fakeRepoImporter reports for one protected branch.
type branchOutcome struct {
	id, branch string
	err        error
}

// fakeRepoImporter records the ImportRepo call and reports branches to its progress.
type fakeRepoImporter struct {
	mu         sync.Mutex
	got        []project.RepoImport
	repo       domain.Repository
	err        error
	importable error
	branches   []branchOutcome
}

func (f *fakeRepoImporter) ImportRepo(
	_ context.Context,
	in project.RepoImport,
	progress project.RepoProgress,
) (domain.Repository, error) {
	f.mu.Lock()
	f.got = append(f.got, in)
	f.mu.Unlock()
	for _, b := range f.branches {
		progress.Workspace(b.id, b.branch, b.err)
	}
	if f.repo.ID == "" {
		return domain.Repository{ID: in.ID, ProjectID: in.ProjectID, Name: in.Name, Path: in.Path}, f.err
	}
	return f.repo, f.err
}

func (f *fakeRepoImporter) CheckRepoImportable(context.Context, string, string) error {
	return f.importable
}

func (f *fakeRepoImporter) calls() []project.RepoImport {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]project.RepoImport(nil), f.got...)
}

func newCreateRouter(
	imp *fakeRepoImporter,
	ops repohandlers.Operations,
	bc *recordingRepoBroadcaster,
) *gin.Engine {
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, ops, bc.push, unusedHome).
		WithStat(statRepoOK).
		WithImporter(imp)
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)
	return r
}

func doPost(r *gin.Engine, target string, body any) *httptest.ResponseRecorder {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, target, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// The add answers with its operation: the repo under the id minted here, then
// one item per protected branch. The repo is announced once it exists.
func TestCreate_AnswersWithTheOperation(t *testing.T) {
	imp := &fakeRepoImporter{branches: []branchOutcome{{id: "w-main", branch: "main"}}}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()
	rec := doPost(newCreateRouter(imp, ops, bc), "/v0/projects/p1/repos", map[string]any{"name": "alpha", "path": "/tmp/repo"})
	var accepted struct {
		Data struct {
			Operation domain.Operation `json:"operation"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &accepted))
	require.Len(t, accepted.Data.Operation.Items, 1, "the repo is named at accept")
	repoID := accepted.Data.Operation.Items[0].Ref.ID

	op := ops.settled(t, rec)
	assert.Equal(t, domain.OpRepoCreate, op.Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityProject, ID: "p1"}, op.Subject)
	assert.Equal(t, "Add alpha", op.Label)
	assert.Equal(t, domain.OpSucceeded, op.State)
	assert.Equal(t, []domain.OperationItem{
		{Ref: domain.EntityRef{Kind: domain.EntityRepo, ID: repoID}, Label: "Add alpha", State: domain.OpSucceeded},
		{Ref: domain.EntityRef{Kind: domain.EntityWorkspace, ID: "w-main"}, Label: "main", State: domain.OpSucceeded},
	}, op.Items)
	assert.Equal(t, []project.RepoImport{{ID: repoID, ProjectID: "p1", Name: "alpha", Path: "/tmp/repo"}}, imp.calls())
	got := bc.next(t)
	assert.Equal(t, repoID, got.ID)
	assert.Empty(t, got.Status)
}

// A protected branch that could not be set up fails its item with the reason,
// and the operation as partial; the repo itself was added.
func TestCreate_AFailedProtectedBranchIsAPartialFailure(t *testing.T) {
	imp := &fakeRepoImporter{branches: []branchOutcome{
		{id: "w-main", branch: "main", err: errors.New("mkdir: not a directory")},
		{id: "w-dev", branch: "dev"},
	}}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()

	op := ops.settled(t, doPost(newCreateRouter(imp, ops, bc), "/v0/projects/p1/repos", map[string]any{"name": "alpha", "path": "/tmp/repo"}))
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Equal(t, "partial", string(op.Error.Code))
	require.Len(t, op.Items, 3)
	assert.Equal(t, domain.OpSucceeded, op.Items[0].State, "the repo was added")
	assert.Equal(t, domain.OpFailed, op.Items[1].State)
	require.NotNil(t, op.Items[1].Error)
	assert.Contains(t, op.Items[1].Error.Message, "not a directory")
	assert.Equal(t, domain.OpSucceeded, op.Items[2].State)
	bc.next(t)
}

// A failed import fails the repo item with its reason and announces nothing.
func TestCreate_AFailedImportFailsTheRepoItem(t *testing.T) {
	imp := &fakeRepoImporter{err: errStore}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()

	op := ops.settled(t, doPost(newCreateRouter(imp, ops, bc), "/v0/projects/p1/repos", map[string]any{"name": "alpha", "path": "/tmp/repo"}))
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Contains(t, op.Error.Message, errStore.Error())
	require.Len(t, op.Items, 1)
	assert.Equal(t, domain.OpFailed, op.Items[0].State)
	assertNoBroadcast(t, bc)
}

// Re-adding a folder the project has returns the existing repo, and the
// item names it rather than the id minted for the request.
func TestCreate_ReAddingAFolderNamesTheExistingRepo(t *testing.T) {
	imp := &fakeRepoImporter{repo: domain.Repository{ID: "r-old", ProjectID: "p1"}}
	ops, bc := newRecordedOps(t), newRecordingRepoBroadcaster()

	op := ops.settled(t, doPost(newCreateRouter(imp, ops, bc), "/v0/projects/p1/repos", map[string]any{"name": "alpha", "path": "/tmp/repo"}))
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityRepo, ID: "r-old"}, op.Items[0].Ref)
	assert.Equal(t, "r-old", bc.next(t).ID)
}

// A folder another project owns is refused with the usecase's message, and
// nothing is imported.
func TestCreate_AlreadyImported_409(t *testing.T) {
	imp := &fakeRepoImporter{importable: fmt.Errorf("%w in the project %q", project.ErrRepoAlreadyImported, "Cloned")}
	bc := newRecordingRepoBroadcaster()

	rec := doPost(newCreateRouter(imp, newRecordedOps(t), bc), "/v0/projects/p2/repos", map[string]any{"name": "alpha", "path": "/tmp/repo"})
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Contains(t, rec.Body.String(), "Cloned", "the refusal names the project that has the folder")
	assert.Empty(t, imp.calls())
	assertNoBroadcast(t, bc)
}

func TestCreate_PathMissing_4xx(t *testing.T) {
	imp := &fakeRepoImporter{}
	r := gin.New()
	h := repohandlers.NewWithDeps(&fakeStore{}, nil, nil, newRecordedOps(t), nil, unusedHome).
		WithStat(func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }).
		WithImporter(imp)
	r.Group("/v0/projects/:projectId").POST("/repos", h.Create)

	rec := doPost(r, "/v0/projects/p1/repos", map[string]any{"name": "alpha", "path": "/missing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, imp.calls())
}

func TestCreate_MissingName_4xx(t *testing.T) {
	rec := doPost(newCreateRouter(&fakeRepoImporter{}, newRecordedOps(t), newRecordingRepoBroadcaster()),
		"/v0/projects/p1/repos", map[string]any{"path": "/tmp/repo"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreate_BadJSON_4xx(t *testing.T) {
	r := newCreateRouter(&fakeRepoImporter{}, newRecordedOps(t), newRecordingRepoBroadcaster())
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v0/projects/p1/repos", bytes.NewReader([]byte("{not-json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// traversalNames is the set of repository names that must never reach a
// filepath.Join: a repo with no parseable remote derives its worktree slug
// from its name, so these would land worktrees outside the crowbar home.
var traversalNames = []struct{ name, value string }{
	{"parent traversal", "../../../../tmp/pwned"},
	{"leading separator", "/etc/passwd"},
	{"embedded separator", "a/b"},
	{"windows separator", `a\b`},
	{"dot", "."},
	{"dot dot", ".."},
	{"dots only", "..."},
}

func TestCreate_RejectsNamesThatEscapeTheCrowbarHome(t *testing.T) {
	for _, c := range traversalNames {
		t.Run(c.name, func(t *testing.T) {
			imp, bc := &fakeRepoImporter{}, newRecordingRepoBroadcaster()
			rec := doPost(newCreateRouter(imp, newRecordedOps(t), bc), "/v0/projects/p1/repos",
				map[string]any{"name": c.value, "path": "/tmp/repo"})

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, imp.calls())
			assertNoBroadcast(t, bc)
		})
	}
}
