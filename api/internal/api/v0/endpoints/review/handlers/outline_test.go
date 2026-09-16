package handlers_test

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type outlineEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Files []gitdomain.FileOutline `json:"files"`
	} `json:"data"`
}

func getOutline(
	r http.Handler,
	acceptEncoding string,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/chats/chat1/review/outline", http.NoBody)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	r.ServeHTTP(rec, req)
	return rec
}

func TestReviewHandlers_GetOutline(t *testing.T) {
	rec := getOutline(newRouter(), "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var env outlineEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Len(t, env.Data.Files, 2)
	assert.Equal(t, "a.go", env.Data.Files[0].Path)
	require.Len(t, env.Data.Files[0].Hunks, 1)
	assert.Equal(t, 6, env.Data.Files[0].Hunks[0].NewLines)
	assert.True(t, env.Data.Files[1].IsBinary)
}

// An empty outline must serialise as [] so the client can index it without a
// null check, exactly as /review/files does.
func TestReviewHandlers_GetOutline_EmptyIsArrayNotNull(t *testing.T) {
	uc := stubUsecase{
		outline: func(_ context.Context, _, _ string) ([]gitdomain.FileOutline, error) { return nil, nil },
	}
	rec := getOutline(routerFor(uc), "")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"files":[]`)
}

// The outline of a million-line diff is megabytes of JSON whose field names
// repeat once per hunk, so the wire cost collapses under gzip. The client must
// get byte-identical content either way.
func TestReviewHandlers_GetOutline_GzipsWhenTheClientAcceptsIt(t *testing.T) {
	plain := getOutline(newRouter(), "")
	require.Equal(t, http.StatusOK, plain.Code)

	rec := getOutline(newRouter(), "gzip, deflate")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")

	zr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer func() { _ = zr.Close() }()
	decoded, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.JSONEq(t, plain.Body.String(), string(decoded))
}

// "gzip;q=0" is an explicit refusal, not an offer.
func TestReviewHandlers_GetOutline_HonoursAZeroQualityRefusal(t *testing.T) {
	rec := getOutline(newRouter(), "gzip;q=0, identity")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Content-Encoding"))

	var env outlineEnvelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
}

// A weighted offer is still an offer.
func TestReviewHandlers_GetOutline_AcceptsAWeightedOffer(t *testing.T) {
	rec := getOutline(newRouter(), "gzip;q=0.5")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
}
