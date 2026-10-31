package stream_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func TestParseScope_AcceptsEveryKind(t *testing.T) {
	cases := map[string]stream.Scope{
		"projects": stream.Projects,
		"chat:c1":  {Kind: "chat", Key: "c1"},
		"ws:w1":    {Kind: "ws", Key: "w1"},
		"git:w1":   {Kind: "git", Key: "w1"},
		"files:w1": {Kind: "files", Key: "w1"},
		"lsp:c1":   {Kind: "lsp", Key: "c1"},
		// A host path is the key as it is: it may hold the separator itself.
		"doc:/h/a:b.md": {Kind: "doc", Key: "/h/a:b.md"},
	}
	for raw, want := range cases {
		got, err := stream.ParseScope(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
		assert.Equal(t, raw, got.String(), "a scope prints as it was spelled")
	}
}

func TestParseScope_RefusesAnUnknownKindOrAMissingKey(t *testing.T) {
	for _, raw := range []string{"", "nope", "nope:x", "chat", "chat:", "ws:", "doc:", "projects:p1", ":x"} {
		_, err := stream.ParseScope(raw)
		require.Error(t, err, raw)
		assert.Equal(t, problem.InvalidArgument, problem.CodeOf(err), raw)
	}
}
