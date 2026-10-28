package handlers_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat/handlers"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var knownRef = "sha256:" + strings.Repeat("ab", 32)

// fakeBlobs holds one payload, under knownRef of chat-1.
type fakeBlobs struct{}

func (fakeBlobs) ReadBlob(_ context.Context, chatID, ref string) ([]byte, error) {
	switch {
	case !strings.HasPrefix(ref, "sha256:"):
		return nil, fmt.Errorf("blob %q: %w", ref, problem.ErrInvalidArgument)
	case chatID != "chat-1" || ref != knownRef:
		return nil, fmt.Errorf("blob %q: %w", ref, problem.ErrNotFound)
	}
	return []byte(`{"command":"ls"}`), nil
}

// blobAnswer is what GET /chats/:chatId/blobs/:ref answered.
type blobAnswer struct {
	status      int
	contentType string
	body        string
}

func getBlob(t *testing.T, chatID, ref string) blobAnswer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/chats/:chatId/blobs/:ref", handlers.NewBlobs(fakeBlobs{}).Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		srv.URL+"/chats/"+chatID+"/blobs/"+url.PathEscape(ref), http.NoBody)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return blobAnswer{status: resp.StatusCode, contentType: resp.Header.Get("Content-Type"), body: string(body)}
}

func TestBlobs_ReturnsTheBytesForAKnownRef(t *testing.T) {
	got := getBlob(t, "chat-1", knownRef)

	require.Equal(t, http.StatusOK, got.status)
	assert.Equal(t, "text/plain; charset=utf-8", got.contentType)
	assert.JSONEq(t, `{"command":"ls"}`, got.body)
}

func TestBlobs_MalformedRefIs400(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, getBlob(t, "chat-1", strings.Repeat("ab", 32)).status)
}

func TestBlobs_UnknownRefIs404(t *testing.T) {
	assert.Equal(t, http.StatusNotFound, getBlob(t, "chat-1", "sha256:"+strings.Repeat("cd", 32)).status)
}

func TestBlobs_UnknownChatIs404(t *testing.T) {
	assert.Equal(t, http.StatusNotFound, getBlob(t, "nope", knownRef).status)
}
