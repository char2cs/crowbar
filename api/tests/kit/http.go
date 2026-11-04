//go:build integration

package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// GET issues a GET request to path and returns the response.
func (e *Env) GET(t *testing.T, path string) *http.Response {
	t.Helper()
	return e.do(t, http.MethodGet, path, nil)
}

// POST issues a POST request with a JSON body to path and returns the response.
func (e *Env) POST(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.do(t, http.MethodPost, path, body)
}

// PUT issues a PUT request with a JSON body.
func (e *Env) PUT(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.do(t, http.MethodPut, path, body)
}

// PATCH issues a PATCH request with a JSON body.
func (e *Env) PATCH(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.do(t, http.MethodPatch, path, body)
}

// DELETE issues a DELETE request.
func (e *Env) DELETE(t *testing.T, path string) *http.Response {
	t.Helper()
	return e.do(t, http.MethodDelete, path, nil)
}

// DELETEJ issues a DELETE request with a JSON body.
func (e *Env) DELETEJ(t *testing.T, path string, body any) *http.Response {
	t.Helper()
	return e.do(t, http.MethodDelete, path, body)
}

// do sends one request; a nil body sends none, anything else is sent as JSON.
func (e *Env) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, e.URL+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	require.NoError(t, err)
	return resp
}

// DecodeEnvData decodes the data field of a query envelope
// ({"success":true,"data":{...}}) into dest. It closes the response body.
func DecodeEnvData(
	t *testing.T,
	resp *http.Response,
	dest any,
) {
	t.Helper()
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &env), "DecodeEnvData body: %s", string(raw))
	require.NoError(t, json.Unmarshal(env.Data, dest), "DecodeEnvData.data body: %s", string(raw))
}

// RequireStatus fails the test if the response status doesn't match.
func RequireStatus(
	t *testing.T,
	resp *http.Response,
	want int,
) {
	t.Helper()
	if resp.StatusCode != want {
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		t.Fatalf(
			"expected status %d got %d: %s",
			want,
			resp.StatusCode,
			string(body),
		)
	}
}

func wsURL(
	base string,
	path string,
) string {
	return "ws" + strings.TrimPrefix(base, "http") + path
}

// MustJSON marshals v to JSON or panics if marshalling fails.
func MustJSON(
	v any,
) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("MustJSON: %v", err))
	}
	return string(b)
}
