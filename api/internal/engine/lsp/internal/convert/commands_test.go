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

func TestConvertCommands_AMalformedCommandIsDroppedAndTheRestKept(t *testing.T) {
	actions := ClientCodeActions("/w", json.RawMessage(`[
		{"title":"Bad","command":{"title":"t","command":7}},
		{"title":"Bare","command":7},
		{"title":"Tidy","command":{"title":"t","command":"gopls.tidy"}}
	]`), runsOnly("gopls.tidy"))
	assert.JSONEq(t, `[{"title":"Tidy","command":{"title":"t","command":"gopls.tidy"}}]`, string(actions))

	lenses := ClientLenses(json.RawMessage(`[
		{"range":{"a":1},"command":{"title":"t","command":7}},
		{"range":{"a":2},"command":{"title":5,"command":"x"}},
		{"range":{"a":3},"command":{"title":"run","command":"gopls.test"}}
	]`), runsOnly("gopls.test"))
	assert.JSONEq(t, `[{"range":{"a":3},"command":{"title":"run","command":"gopls.test"}}]`, string(lenses))

	resolved := ClientLenses(json.RawMessage(`{"range":{},"command":{"title":"t","command":7}}`), runsOnly())
	assert.JSONEq(t, `null`, string(resolved))
}
