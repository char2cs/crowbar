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

	chathandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chats/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type fakeLifecycle struct {
	listChats []domain.Chat
	listErr   error
	gotListWs string

	created      domain.Chat
	createErr    error
	gotCreateWs  string
	gotCreateTit string

	forked    domain.Chat
	forkErr   error
	gotForkID string

	renamed      domain.Chat
	renameErr    error
	gotRenameID  string
	gotRenameTit string

	deleteErr   error
	gotDeleteID string
}

func (f *fakeLifecycle) ListChatsByWorkspace(
	_ context.Context,
	wsID string,
) ([]domain.Chat, error) {
	f.gotListWs = wsID
	return f.listChats, f.listErr
}

func (f *fakeLifecycle) CreateChat(
	_ context.Context,
	_ string,
	wsID string,
	title string,
	_ time.Time,
) (domain.Chat, error) {
	f.gotCreateWs = wsID
	f.gotCreateTit = title
	return f.created, f.createErr
}

func (f *fakeLifecycle) ForkChat(
	_ context.Context,
	parentID string,
	_ time.Time,
) (domain.Chat, error) {
	f.gotForkID = parentID
	return f.forked, f.forkErr
}

func (f *fakeLifecycle) RenameChat(
	_ context.Context,
	id string,
	title string,
) (domain.Chat, error) {
	f.gotRenameID = id
	f.gotRenameTit = title
	return f.renamed, f.renameErr
}

func (f *fakeLifecycle) DeleteChat(
	_ context.Context,
	id string,
	_ time.Time,
) error {
	f.gotDeleteID = id
	return f.deleteErr
}

func newRouter(
	chats chathandlers.Lifecycle,
) *gin.Engine {
	r := gin.New()
	h := chathandlers.New(chats, func() time.Time { return time.Unix(1000, 0) })
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/chats", h.List)
	rg.POST("/workspaces/:wsId/chats", h.Create)
	rg.POST("/chats/:id/fork", h.Fork)
	rg.PATCH("/chats/:id", h.Rename)
	rg.DELETE("/chats/:id", h.Delete)
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
	assert.NotNil(t, chathandlers.New(&fakeLifecycle{}, time.Now))
}
