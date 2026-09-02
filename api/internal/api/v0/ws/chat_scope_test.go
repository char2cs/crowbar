package ws

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// chatScopeCtx builds a gin context carrying exactly the given path params.
func chatScopeCtx(params gin.Params) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Params = params
	return c
}

// TestClientScope_ChatRouteResolvesToTheBareChatID pins the fallback that makes
// snapshot-on-subscribe work for the flat /v0/chats/:chatId routes.
//
// It matters because the snapshot path is the ONE consumer of clientScope that
// FlatNamespace does not spare: Broadcaster.Handle passes this string straight
// to StreamDef.Snapshot. A chat route binds no projectId/repoId/wsId, so
// without the fallback the scope would be "" — and terminalsSnapshot answers
// "" with nil. The live predicate would still scope correctly via its chatId
// Filter, so the failure would appear only as "a reconnecting client gets no
// initial session list", which is exactly the kind of silent gap that survives
// a casual smoke test.
func TestClientScope_ChatRouteResolvesToTheBareChatID(t *testing.T) {
	scope := clientScope(chatScopeCtx(gin.Params{{Key: "chatId", Value: "chat-1"}}))

	assert.Equal(t, "chat-1", scope)
}

// TestClientScope_HierarchicalRouteIsUnchangedByTheChatFallback proves the
// fallback is a fallback: every route that still binds the hierarchical params
// keeps the exact scope it had before terminals moved.
func TestClientScope_HierarchicalRouteIsUnchangedByTheChatFallback(t *testing.T) {
	cases := []struct {
		name   string
		params gin.Params
		want   string
	}{
		{
			name: "workspace level",
			params: gin.Params{
				{Key: "projectId", Value: "p"},
				{Key: "repoId", Value: "r"},
				{Key: "wsId", Value: "w"},
			},
			want: "p/r/w",
		},
		{
			name: "repo level",
			params: gin.Params{
				{Key: "projectId", Value: "p"},
				{Key: "repoId", Value: "r"},
			},
			want: "p/r",
		},
		{
			name:   "project level",
			params: gin.Params{{Key: "projectId", Value: "p"}},
			want:   "p",
		},
		{
			// A route binding BOTH keeps its hierarchical scope: the chat id is
			// consulted only after the hierarchical segments come back empty, so
			// adding the fallback cannot re-scope an existing stream.
			name: "hierarchical wins over a chat id",
			params: gin.Params{
				{Key: "projectId", Value: "p"},
				{Key: "repoId", Value: "r"},
				{Key: "wsId", Value: "w"},
				{Key: "chatId", Value: "chat-1"},
			},
			want: "p/r/w",
		},
		{
			name:   "neither shape present matches all",
			params: gin.Params{},
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, clientScope(chatScopeCtx(tc.params)))
		})
	}
}
