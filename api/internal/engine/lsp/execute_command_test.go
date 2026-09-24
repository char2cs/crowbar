package lsp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteCommand_ForwardsTheCommandAndRelativizesItsEdits(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"ok":true}`))
	fake.cmdEdits = []json.RawMessage{
		json.RawMessage(`{"changes":{"file:///tree/go.mod":[]}}`),
	}
	e := buildEngine(t, fake)

	args := json.RawMessage(`[{"URI":"file:///tree/go.mod"}]`)
	got, err := e.ExecuteCommand(context.Background(), ws, tree, goF, "gopls.tidy", args)
	require.NoError(t, err)

	assert.JSONEq(t, `{"ok":true}`, string(got.Result))
	require.Len(t, got.Edits, 1)
	assert.JSONEq(t, `{"changes":{"go.mod":[]}}`, string(got.Edits[0]))
	params := as[map[string]any](t, fake.requests()[0].params)
	assert.Equal(t, "gopls.tidy", params["command"])
	assert.JSONEq(t, string(args), string(as[json.RawMessage](t, params["arguments"])))
}

func TestExecuteCommand_WithoutArgumentsSendsNone(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	got, err := e.ExecuteCommand(context.Background(), ws, tree, goF, "x.run", nil)
	require.NoError(t, err)
	assert.Empty(t, got.Edits)
	assert.NotContains(t, as[map[string]any](t, fake.requests()[0].params), "arguments")
}
