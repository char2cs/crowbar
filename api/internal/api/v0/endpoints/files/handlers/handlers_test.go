package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	filehandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/files/handlers"
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeFiles struct {
	tree    []domain.FileNode
	treeErr error
	gotTree struct {
		ws   string
		path string
	}

	content    domain.FileContent
	contentErr error
	gotRead    struct {
		ws   string
		path string
	}

	writeErr error
	gotWrite struct {
		ws      string
		path    string
		content string
	}

	createFileErr error
	gotCreateFile struct {
		ws   string
		path string
	}

	createDirErr error
	gotCreateDir struct {
		ws   string
		path string
	}

	renameErr error
	gotRename struct {
		ws      string
		oldPath string
		newPath string
	}

	deleteErr error
	gotDelete struct {
		ws   string
		path string
	}
}

func (f *fakeFiles) Tree(
	_ context.Context,
	wsID string,
	dirPath string,
	_ file.FileStatusProvider,
) ([]domain.FileNode, error) {
	f.gotTree.ws = wsID
	f.gotTree.path = dirPath
	return f.tree, f.treeErr
}

func (f *fakeFiles) ReadContent(
	_ context.Context,
	wsID string,
	filePath string,
) (domain.FileContent, error) {
	f.gotRead.ws = wsID
	f.gotRead.path = filePath
	return f.content, f.contentErr
}

func (f *fakeFiles) WriteContent(
	_ context.Context,
	wsID string,
	filePath string,
	content string,
	_ time.Time,
) error {
	f.gotWrite.ws = wsID
	f.gotWrite.path = filePath
	f.gotWrite.content = content
	return f.writeErr
}

func (f *fakeFiles) CreateFile(
	_ context.Context,
	wsID string,
	filePath string,
	_ time.Time,
) error {
	f.gotCreateFile.ws = wsID
	f.gotCreateFile.path = filePath
	return f.createFileErr
}

func (f *fakeFiles) CreateDir(
	_ context.Context,
	wsID string,
	dirPath string,
	_ time.Time,
) error {
	f.gotCreateDir.ws = wsID
	f.gotCreateDir.path = dirPath
	return f.createDirErr
}

func (f *fakeFiles) Rename(
	_ context.Context,
	wsID string,
	oldPath string,
	newPath string,
	_ time.Time,
) error {
	f.gotRename.ws = wsID
	f.gotRename.oldPath = oldPath
	f.gotRename.newPath = newPath
	return f.renameErr
}

func (f *fakeFiles) Delete(
	_ context.Context,
	wsID string,
	filePath string,
	_ time.Time,
) error {
	f.gotDelete.ws = wsID
	f.gotDelete.path = filePath
	return f.deleteErr
}

func newRouter(
	files filehandlers.Files,
) *gin.Engine {
	r := gin.New()
	h := filehandlers.New(files, func() time.Time { return time.Unix(1000, 0) })
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/files/tree", h.Tree)
	rg.GET("/workspaces/:wsId/files/content", h.ReadContent)
	rg.PUT("/workspaces/:wsId/files/content", h.SaveContent)
	rg.POST("/workspaces/:wsId/files", h.Create)
	rg.PATCH("/workspaces/:wsId/files", h.Rename)
	rg.DELETE("/workspaces/:wsId/files", h.Delete)
	return r
}

func do(
	r *gin.Engine,
	method string,
	target string,
	body string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestNew(
	t *testing.T,
) {
	assert.NotNil(t, filehandlers.New(&fakeFiles{}, time.Now))
}
