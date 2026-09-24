package lsp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
)

func TestExecuteCommand_ForwardsTheCommandAndRelativizesItsEdits(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`{"ok":true}`))
	fake.commands = map[string]bool{"gopls.tidy": true}
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
	fake.commands = map[string]bool{"x.run": true}
	e := buildEngine(t, fake)

	got, err := e.ExecuteCommand(context.Background(), ws, tree, goF, "x.run", nil)
	require.NoError(t, err)
	assert.Empty(t, got.Edits)
	assert.NotContains(t, as[map[string]any](t, fake.requests()[0].params), "arguments")
}

// A command the server never declared is the client's (e.g. a "show
// references" lens); forwarding it would only earn a server error.
func TestExecuteCommand_RejectsACommandTheServerDoesNotRun(t *testing.T) {
	fake := newFakeServer(nil)
	e := buildEngine(t, fake)

	_, err := e.ExecuteCommand(context.Background(), ws, tree, goF, "java.show.references", nil)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Empty(t, fake.requests())
}

func TestCodeLens_OnlyOffersCommandsTheServerRuns(t *testing.T) {
	fake := newFakeServer(json.RawMessage(`[
		{"range":{},"command":{"title":"run test","command":"gopls.test","arguments":[1]}},
		{"range":{},"command":{"title":"3 references","command":"java.show.references","arguments":[2]}},
		{"range":{},"data":7}
	]`))
	fake.commands = map[string]bool{"gopls.test": true}
	e := buildEngine(t, fake)

	got, err := e.CodeLens(context.Background(), ws, tree, goF)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"range":{},"command":{"title":"run test","command":"gopls.test","arguments":[1]}},
		{"range":{},"command":{"title":"3 references","command":""}},
		{"range":{},"data":7}
	]`, string(got))

	fake.result = json.RawMessage(`{"range":{},"command":{"title":"2 impls","command":"client.impls"}}`)
	got, err = e.CodeLensResolve(context.Background(), ws, tree, goF, json.RawMessage(`{"range":{},"data":7}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"range":{},"command":{"title":"2 impls","command":""}}`, string(got))
}
