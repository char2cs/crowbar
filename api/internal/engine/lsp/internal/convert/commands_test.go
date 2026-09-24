package convert

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
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
	got := ClientCodeActions("/w", raw, runsOnly("gopls.tidy"))
	assert.JSONEq(t, `[
		{"title":"Tidy","command":"gopls.tidy","arguments":[]},
		{"title":"Edit+client","edit":{"changes":{"a.go":[]}}},
		{"title":"Edit+server","edit":{"changes":{}},"command":{"title":"t","command":"gopls.tidy"}}
	]`, string(got))
}

func TestClientLenses_PassesThroughWhatDoesNotDecode(t *testing.T) {
	for _, raw := range []string{`null`, `"x"`, `42`} {
		assert.Equal(t, raw, string(ClientLenses(json.RawMessage(raw), runsOnly())))
	}
}
