package v0

import (
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/ws"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestFilesDef_Lambdas(t *testing.T) {
	def := filesDef(nil)
	evt := domain.FileChangeEvent{WsID: "w1", Path: "a.go"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "a.go")

	// TWO filters for the two live mounts: the home route resolves wsId, the
	// chat-scoped one a bound chatId; each client activates only its own.
	require.Len(t, def.Filters, 2)
	assert.Equal(t, "wsId", def.Filters[0].Param)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
	assert.Equal(t, "chatId", def.Filters[1].Param)
	assert.Equal(t, "w1", def.Filters[1].Extract(evt))
	assert.NotNil(t, def.Filters[1].Resolve)
	assert.False(t, def.Filters[1].Required)
}

// TestFilesDef_CarriesNoSnapshot pins a real design answer rather than an
// absence nobody chose: a file-change event is NEWS, not state. A connecting
// client has already fetched the tree over REST, so there is nothing to replay
// — which is why the chat-scoped move needed no chat-resolving snapshot source
// of the kind gitSnapshot grew. If a snapshot is ever added here it must answer
// the BARE chat scope too (see gitSnapshot), and this is the test that will say
// so.
func TestFilesDef_CarriesNoSnapshot(t *testing.T) {
	assert.Nil(t, filesDef(nil).Snapshot)
}

func TestDocumentsDef_AClientHoldsAndHearsItsOwnPathAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	held, released := make(chan string, 1), make(chan string, 1)
	def := documentsDef()
	def.Admit = func(scope string) error {
		held <- scope
		return nil
	}
	def.OnUnsubscribe = func(scope string) { released <- scope }
	b := ws.NewBroadcaster(def)
	r := gin.New()
	r.GET("/v0/documents/ws", b.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	conn := dialWSAt(t, srv, "/v0/documents/ws?path=%2Fd%2Fa.md")
	assert.Equal(t, "/d/a.md", <-held)
	b.Push(domain.FileChangeEvent{Type: domain.FileChangeModified, Authority: domain.DocumentAuthorityHost, Path: "/d/b.md"})
	b.Push(domain.FileChangeEvent{Type: domain.FileChangeCreated, Authority: domain.DocumentAuthorityHost, Path: "/d/a.md"})

	assert.Equal(t, map[string]any{"type": "created", "authority": "host", "path": "/d/a.md", "wsId": ""}, readJSON(t, conn))
	require.NoError(t, conn.Close())
	assert.Equal(t, "/d/a.md", <-released)
}

func TestContainer_PushFile_RoutesHostChangesToDocumentsAndTheRestToFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := New(newAppForSnapshot(t), nil)
	t.Cleanup(c.Close)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	path := filepath.Join(dir, "a.md")
	r := gin.New()
	r.GET("/files", c.files.Handle)
	r.GET("/documents", c.documents.Handle)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	files := dialWSAt(t, srv, "/files")
	document := dialWSAt(t, srv, "/documents?path="+url.QueryEscape(path))

	c.PushFile(domain.FileChangeEvent{Type: domain.FileChangeModified, Authority: domain.DocumentAuthorityWorkspace, Path: path, WsID: "w1"})
	c.PushFile(domain.FileChangeEvent{Type: domain.FileChangeModified, Authority: domain.DocumentAuthorityHost, Path: path})
	c.PushFile(domain.FileChangeEvent{Type: domain.FileChangeModified, Authority: domain.DocumentAuthorityWorkspace, Path: "b.go", WsID: "w2"})

	assert.Equal(t, "host", readJSON(t, document)["authority"], "a workspace change never reaches a document's holder")
	assert.Equal(t, "w1", readJSON(t, files)["wsId"])
	assert.Equal(t, "b.go", readJSON(t, files)["path"], "a host change never reaches a worktree's clients")
}
