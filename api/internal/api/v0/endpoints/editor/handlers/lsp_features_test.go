package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestSignatureHelp_200Raw(t *testing.T) {
	r := newRouter(&fakeLSP{signatureHelp: json.RawMessage(`{"signatures":[]}`)}, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/signatureHelp", map[string]any{
		"path":     "main.go",
		"position": map[string]int{"line": 3, "character": 9},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"signatures":[]}`, string(decode(t, rec).Data))
}

func TestCodeLensAndResolve_200Raw(t *testing.T) {
	lsp := &fakeLSP{
		codeLens:     json.RawMessage(`[{"range":{},"data":1}]`),
		resolvedLens: json.RawMessage(`{"range":{},"command":{"title":"2 refs","command":""}}`),
	}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/codeLens", map[string]any{"path": "a.ts"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[{"range":{},"data":1}]`, string(decode(t, rec).Data))

	rec = do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/codeLensResolve", map[string]any{
		"path": "a.ts",
		"lens": map[string]any{"range": map[string]any{}, "data": 1},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"range":{},"data":1}`, string(lsp.gotLens))
	assert.Contains(t, string(decode(t, rec).Data), "2 refs")
}

func TestFormatting_ForwardsOptions(t *testing.T) {
	lsp := &fakeLSP{formatting: json.RawMessage(`[]`)}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/formatting", map[string]any{
		"path":    "main.go",
		"options": map[string]any{"tabSize": 2, "insertSpaces": true},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, domlsp.FormattingOptions{TabSize: 2, InsertSpaces: true}, lsp.formatOptions)
}

func TestSemanticTokens_ForwardsThePreviousResultID(t *testing.T) {
	lsp := &fakeLSP{semanticTokens: json.RawMessage(`{"resultId":"2","edits":[]}`)}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/semanticTokens", map[string]any{
		"path": "main.go", "previousResultId": "1",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "1", lsp.gotResultID)
	assert.JSONEq(t, `{"resultId":"2","edits":[]}`, string(decode(t, rec).Data))

	rec = do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/semanticTokens", map[string]any{})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestSemanticTokensRange_ForwardsTheRange(t *testing.T) {
	lsp := &fakeLSP{}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/semanticTokensRange", map[string]any{
		"path":  "main.go",
		"range": map[string]any{"start": map[string]int{"line": 4}, "end": map[string]int{"line": 60}},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 60, lsp.gotRange.End.Line)
	assert.JSONEq(t, `null`, string(decode(t, rec).Data))
}

func TestExecuteCommand_ReturnsTheEditsToApply(t *testing.T) {
	lsp := &fakeLSP{command: domlsp.CommandResult{
		Result: json.RawMessage(`null`),
		Edits:  []json.RawMessage{json.RawMessage(`{"changes":{"go.mod":[]}}`)},
	}}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/executeCommand", map[string]any{
		"path": "main.go", "command": "gopls.tidy", "arguments": []any{map[string]any{"x": 1}},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "gopls.tidy", lsp.gotCommand)
	assert.JSONEq(t, `[{"x":1}]`, string(lsp.gotArguments))
	assert.JSONEq(t, `{"result":null,"edits":[{"changes":{"go.mod":[]}}]}`, string(decode(t, rec).Data))

	rec = do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/executeCommand", map[string]any{"path": "main.go"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCodeAction_ForwardsDiagnostics(t *testing.T) {
	lsp := &fakeLSP{codeAction: json.RawMessage(`[]`)}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/codeAction", map[string]any{
		"path":        "main.go",
		"range":       map[string]any{},
		"diagnostics": []map[string]any{{"message": "unused"}},
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[{"message":"unused"}]`, string(lsp.codeActionDiag))
}

func TestStatusAndRestart_ReportServerState(t *testing.T) {
	lsp := &fakeLSP{status: domlsp.ServerStatus{
		LanguageID: "go", Command: "gopls", State: domlsp.ServerRunning,
	}}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodGet, "/v0/chats/ws1/lsp/status?path=main.go", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"languageId":"go","command":"gopls","state":"running"}`,
		string(decode(t, rec).Data))

	rec = do(t, r, http.MethodGet, "/v0/chats/ws1/lsp/status", nil)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/restart", map[string]any{"path": "main.go"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, lsp.restartCalls)
	assert.Contains(t, string(decode(t, rec).Data), `"running"`)
}

func TestDidSave_200(t *testing.T) {
	lsp := &fakeLSP{}
	r := newRouter(lsp, &fakeGit{})

	rec := do(t, r, http.MethodPost, "/v0/chats/ws1/lsp/didSave", map[string]any{"path": "main.go"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, lsp.didSaveCalls)
}
