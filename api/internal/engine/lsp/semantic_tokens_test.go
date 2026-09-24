package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	"github.com/char2cs/crowbar/api/internal/engine/lsp/internal/semtok"
)

// supportFor builds a server's semantic-token support whose legend is
// [keyword, function] — canonical indices 15 and 12.
func supportFor(
	t *testing.T,
	options string,
) semtok.Support {
	t.Helper()
	return semtok.FromCapabilities(json.RawMessage(`{"semanticTokensProvider":{` +
		`"legend":{"tokenTypes":["keyword","function"],"tokenModifiers":[]},` + options + `}}`))
}

func TestSemanticTokens_FullIsRemappedToTheCanonicalLegend(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"resultId":"1","data":[0,0,4,0,0, 0,5,3,1,0]}`))
	fake.semTok = supportFor(t, `"full":true`)
	e := buildEngine(t, fake)

	got, err := e.SemanticTokens(context.Background(), ws, tree, goF, "")
	require.NoError(t, err)
	assert.JSONEq(t, `{"resultId":"1","data":[0,0,4,15,0,0,5,3,12,0]}`, string(got))
	assert.Equal(t, "textDocument/semanticTokens/full", fake.requests()[0].method)
}

func TestSemanticTokens_DeltaWhenTheServerSupportsIt(t *testing.T) {
	fake := newFakeServer(nil)
	fake.byMethod = map[string]json.RawMessage{
		"textDocument/semanticTokens/full/delta": json.RawMessage(
			`{"resultId":"2","edits":[{"start":3,"deleteCount":1,"data":[1]}]}`),
	}
	fake.semTok = supportFor(t, `"full":{"delta":true}`)
	e := buildEngine(t, fake)

	got, err := e.SemanticTokens(context.Background(), ws, tree, goF, "1")
	require.NoError(t, err)
	assert.JSONEq(t, `{"resultId":"2","edits":[{"start":3,"deleteCount":1,"data":[12]}]}`, string(got))
	params := as[map[string]any](t, fake.requests()[0].params)
	assert.Equal(t, "1", params["previousResultId"])
}

func TestSemanticTokens_WithoutDeltaSupportAsksForFull(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"data":[]}`))
	fake.semTok = supportFor(t, `"full":true`)
	e := buildEngine(t, fake)

	_, err := e.SemanticTokens(context.Background(), ws, tree, goF, "1")
	require.NoError(t, err)
	require.Len(t, fake.requests(), 1)
	assert.Equal(t, "textDocument/semanticTokens/full", fake.requests()[0].method)
}

func TestSemanticTokens_MisalignedDeltaFallsBackToFull(t *testing.T) {
	fake := newFakeServer(nil)
	fake.byMethod = map[string]json.RawMessage{
		"textDocument/semanticTokens/full/delta": json.RawMessage(
			`{"edits":[{"start":0,"deleteCount":0,"data":[1,2]},{"start":10,"deleteCount":2}]}`),
		"textDocument/semanticTokens/full": json.RawMessage(`{"resultId":"3","data":[0,0,1,1,0]}`),
	}
	fake.semTok = supportFor(t, `"full":{"delta":true}`)
	e := buildEngine(t, fake)

	got, err := e.SemanticTokens(context.Background(), ws, tree, goF, "2")
	require.NoError(t, err)
	assert.JSONEq(t, `{"resultId":"3","data":[0,0,1,12,0]}`, string(got))
}

// A restarted server rejects a resultId it never issued; the answer must be a
// full result, or the editor would resend that id on every edit.
func TestSemanticTokens_RejectedDeltaFallsBackToFull(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"data":[]}`))
	fake.errByMethod = map[string]error{
		"textDocument/semanticTokens/full/delta": errors.New("rpc error -32602: unknown resultId"),
	}
	fake.semTok = supportFor(t, `"full":{"delta":true}`)
	e := buildEngine(t, fake)

	got, err := e.SemanticTokens(context.Background(), ws, tree, goF, "stale")
	require.NoError(t, err)
	assert.JSONEq(t, `{"data":[]}`, string(got))
}

// A server that offers no semantic tokens is never asked for them.
func TestSemanticTokens_UnsupportedAnswersNil(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"data":[1]}`))
	e := buildEngine(t, fake)

	got, err := e.SemanticTokens(context.Background(), ws, tree, goF, "")
	require.NoError(t, err)
	assert.Nil(t, got)
	got, err = e.SemanticTokensRange(context.Background(), ws, tree, goF, domlsp.Range{})
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Empty(t, fake.requests())
}

func TestSemanticTokensRange_ForwardsTheRange(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"data":[2,0,3,0,0]}`))
	fake.semTok = supportFor(t, `"range":true`)
	e := buildEngine(t, fake)

	rng := domlsp.Range{Start: domlsp.Position{Line: 2}, End: domlsp.Position{Line: 40}}
	got, err := e.SemanticTokensRange(context.Background(), ws, tree, goF, rng)
	require.NoError(t, err)
	assert.JSONEq(t, `{"data":[2,0,3,15,0]}`, string(got))
	req := fake.requests()[0]
	assert.Equal(t, "textDocument/semanticTokens/range", req.method)
	assert.Contains(t, as[map[string]any](t, req.params), "range")
}
