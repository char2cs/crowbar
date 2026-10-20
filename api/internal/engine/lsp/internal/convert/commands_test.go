package convert

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runsOnly(names ...string) func(string) bool {
	return func(c string) bool {
		for _, n := range names {
			if n == c {
				return true
			}
		}
		return false
	}
}

func TestClientCodeActions_KeepsOnlyWhatTheEditorCanDo(t *testing.T) {
	raw := json.RawMessage(`[
		{"title":"Tidy","command":"gopls.tidy","arguments":[]},
		{"title":"Client","command":"editor.action.x"},
		{"title":"Edit+client","edit":{"changes":{"file:///w/a.go":[]}},"command":{"title":"t","command":"editor.action.x"}},
		{"title":"Edit+server","edit":{"changes":{}},"command":{"title":"t","command":"gopls.tidy"}},
		{"title":"Nothing"}
	]`)
	got, err := ClientCodeActions("/w", raw, runsOnly("gopls.tidy"))
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"title":"Tidy","command":"gopls.tidy","arguments":[]},
		{"title":"Edit+client","edit":{"changes":{"a.go":[]}}},
		{"title":"Edit+server","edit":{"changes":{}},"command":{"title":"t","command":"gopls.tidy"}}
	]`, string(got))
}

func TestClientLenses_PassesThroughWhatDoesNotDecode(t *testing.T) {
	for _, raw := range []string{`null`, `"x"`, `42`} {
		got, err := ClientLenses(json.RawMessage(raw), runsOnly())
		require.NoError(t, err)
		assert.Equal(t, raw, string(got))
	}
}

func TestConvertCommands_AMalformedArgumentIsAnError(t *testing.T) {
	_, err := ClientCodeActions("/w", json.RawMessage(`[{"title":"x","command":{"title":"t","command":7}}]`), runsOnly())
	require.Error(t, err)

	_, err = ClientLenses(json.RawMessage(`[{"command":{"title":"t","command":7}}]`), runsOnly())
	require.Error(t, err)

	_, err = ClientLenses(json.RawMessage(`{"command":{"title":5,"command":"x"}}`), runsOnly())
	require.Error(t, err)
}
