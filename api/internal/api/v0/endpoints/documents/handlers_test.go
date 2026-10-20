package documents_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/documents"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// recordingDocuments answers every call with err (or the target, resolved as
// the host) and remembers what the handler asked for.
type recordingDocuments struct {
	target    domain.DocumentRef
	transport domain.Transport
	body      [2]string
	err       error
}

func (r *recordingDocuments) ReadDocument(
	_ context.Context,
	target domain.DocumentRef,
	transport domain.Transport,
) (domain.Document, error) {
	r.target, r.transport = target, transport
	if r.err != nil {
		return domain.Document{}, r.err
	}
	return domain.Document{DocumentRef: resolved(target), FileContent: domain.FileContent{Content: "hi"}}, nil
}

func (r *recordingDocuments) WriteDocument(
	_ context.Context,
	target domain.DocumentRef,
	content string,
	encoding string,
	transport domain.Transport,
	_ time.Time,
) (domain.DocumentRef, error) {
	r.target, r.transport, r.body = target, transport, [2]string{content, encoding}
	if r.err != nil {
		return domain.DocumentRef{}, r.err
	}
	return resolved(target), nil
}

func resolved(
	target domain.DocumentRef,
) domain.DocumentRef {
	if target.Authority == "" {
		target.Authority = domain.DocumentAuthorityHost
	}
	return target
}

func newRouter(
	docs documents.Documents,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	documents.Register(r.Group("/v0"), docs)
	return r
}

type envelope struct {
	Error string          `json:"error"`
	Code  string          `json:"code"`
	Data  json.RawMessage `json:"data"`
}

// send makes one request through client and answers its status and envelope.
func send(
	t *testing.T,
	client *http.Client,
	method string,
	url string,
	body string,
) (int, envelope) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, url, strings.NewReader(body))
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	var env envelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	return resp.StatusCode, env
}

// serve answers one request handled in process, with no listener behind it.
func serve(
	t *testing.T,
	r *gin.Engine,
	url string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, url, nil))
	return rec
}

func TestTransport_TCPIsRemote(t *testing.T) {
	docs := &recordingDocuments{}
	srv := httptest.NewServer(newRouter(docs))
	t.Cleanup(srv.Close)

	status, _ := send(t, srv.Client(), http.MethodGet, srv.URL+"/v0/documents/content?path=%2Ftmp%2Fx.md", "")

	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, domain.TransportRemote, docs.transport)
}

func TestTransport_UnixSocketIsLocal(t *testing.T) {
	dir, err := os.MkdirTemp("", "doc")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", filepath.Join(dir, "s"))
	require.NoError(t, err)
	docs := &recordingDocuments{}
	srv := httptest.NewUnstartedServer(newRouter(docs))
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", ln.Addr().String())
		},
	}}

	status, _ := send(t, client, http.MethodGet, "http://unix/v0/documents/content?path=%2Ftmp%2Fx.md", "")

	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, domain.TransportLocal, docs.transport)
}

// TestTransport_NoLocalAddressIsRemote pins the fail-closed default: a request
// that names no listener at all is never treated as the local socket.
func TestTransport_NoLocalAddressIsRemote(t *testing.T) {
	docs := &recordingDocuments{transport: domain.TransportLocal}

	rec := serve(t, newRouter(docs), "/v0/documents/content?path=%2Fx")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, domain.TransportRemote, docs.transport)
}

func TestRead_ParsesWorkspaceOrAbsoluteTargets(t *testing.T) {
	docs := &recordingDocuments{}
	r := newRouter(docs)
	cases := map[string]domain.DocumentRef{
		"/v0/documents/content?workspace=w1&path=src%2Fa.ts": {Authority: domain.DocumentAuthorityWorkspace, WorkspaceID: "w1", Path: "src/a.ts"},
		"/v0/documents/content?path=%2Ftmp%2Fx.md":           {Path: "/tmp/x.md"},
	}
	for url, want := range cases {
		rec := serve(t, r, url)
		require.Equal(t, http.StatusOK, rec.Code, url)
		assert.Equal(t, want, docs.target, url)
	}

	rec := serve(t, r, "/v0/documents/content?workspace=w1")
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a document needs a path")
}

func TestRead_AnswersTheResolvedDocument(t *testing.T) {
	srv := httptest.NewServer(newRouter(&recordingDocuments{}))
	t.Cleanup(srv.Close)

	_, env := send(t, srv.Client(), http.MethodGet, srv.URL+"/v0/documents/content?workspace=w1&path=a.md", "")

	var doc map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &doc))
	assert.Equal(t, map[string]any{"authority": "workspace", "workspaceId": "w1", "path": "a.md", "content": "hi"}, doc)
}

func TestWrite_PassesTheBodyAndAnswersTheResolvedRef(t *testing.T) {
	docs := &recordingDocuments{}
	srv := httptest.NewServer(newRouter(docs))
	t.Cleanup(srv.Close)
	body := `{"workspaceId":"w1","path":"a.bin","content":"AA==","encoding":"base64"}`

	status, env := send(t, srv.Client(), http.MethodPut, srv.URL+"/v0/documents/content", body)

	require.Equal(t, http.StatusOK, status)
	var ref domain.DocumentRef
	require.NoError(t, json.Unmarshal(env.Data, &ref))
	want := domain.DocumentRef{Authority: domain.DocumentAuthorityWorkspace, WorkspaceID: "w1", Path: "a.bin"}
	assert.Equal(t, want, ref)
	assert.Equal(t, want, docs.target)
	assert.Equal(t, [2]string{"AA==", "base64"}, docs.body)
}

// TestRefusals_CarryTheirCodes pins the codes a client tells refusals apart by,
// rather than by message text.
func TestRefusals_CarryTheirCodes(t *testing.T) {
	for err, code := range map[error]string{
		apperr.ErrHostDocumentNeedsLocalTransport: documents.HostDocumentRequiresLocalTransport,
		apperr.ErrDaemonStateDocument:             documents.DaemonStateDocument,
	} {
		srv := httptest.NewServer(newRouter(&recordingDocuments{err: err}))
		status, env := send(t, srv.Client(), http.MethodGet, srv.URL+"/v0/documents/content?path=%2Fx", "")
		srv.Close()

		assert.Equal(t, http.StatusForbidden, status, code)
		assert.Equal(t, code, env.Code)
	}
}
