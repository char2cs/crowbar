package descriptorcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func parsed(t *testing.T, src string) document {
	t.Helper()
	var root yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte(src), &root))
	return document{root: &root}
}

const sample = "id: x\nsession:\n  locate:\n    root: ~/.x\n    glob:\n      - a/{id}.jsonl\n      - b/{id}.jsonl\n"

func TestDocument_LineIsTheKeyOrItemThePathNames(t *testing.T) {
	d := parsed(t, sample)
	assert.Equal(t, 1, d.line("id"))
	assert.Equal(t, 2, d.line("session"), "a block's heading, not its first child")
	assert.Equal(t, 4, d.line("session.locate.root"))
	assert.Equal(t, 7, d.line("session.locate.glob[1]"))
}

func TestDocument_AMissingPathFallsBackToItsNearestAncestor(t *testing.T) {
	d := parsed(t, sample)
	assert.Equal(t, 3, d.line("session.locate.root_env"))
	assert.Equal(t, 5, d.line("session.locate.glob[9]"))
	assert.Zero(t, d.line("nowhere.at.all"))
}

func TestDocument_PathAtNamesTheKeyOnALine(t *testing.T) {
	d := parsed(t, sample)
	assert.Equal(t, "session.locate.root", d.pathAt(4))
	assert.Equal(t, "session.locate.glob", d.pathAt(5))
	assert.Empty(t, d.pathAt(99))
}

func TestErrorLine_ReadsTheDecoderLine(t *testing.T) {
	assert.Equal(t, 12, errorLine("yaml: line 12: field x not found"))
	assert.Zero(t, errorLine("no line here"))
}
