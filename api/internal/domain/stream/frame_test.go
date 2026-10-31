package stream_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// The golden file is the wire contract the web decoder's test copies, so a
// field renamed here fails both sides.
func TestFrame_EveryTypeMarshalsAsTheGoldenFile(t *testing.T) {
	refused := problem.Problem{Code: problem.InvalidArgument, Message: "stream: unknown scope kind \"nope\""}
	frames := []stream.Frame{
		stream.Ready("e1"),
		{T: stream.FrameEntity, Scope: "projects", Kind: string(stream.KindOperation), ID: "op1", V: 7, Data: json.RawMessage(`{"id":"op1"}`)},
		{T: stream.FrameEntity, Scope: "projects", Kind: string(stream.KindOperation), ID: "op1", V: 8, Gone: true},
		stream.Synced("projects"),
		{T: stream.FrameEntity, Scope: "ws:w1", Kind: string(stream.KindReviewThread), ID: "th1", V: 9, Data: json.RawMessage(`{"id":"th1"}`)},
		{T: stream.FrameSignal, Scope: "git:w1", Kind: "git_status", Data: json.RawMessage(`{"branch":"main"}`)},
		stream.Closed("nope:x", stream.ReasonRefused, &refused),
		stream.Closed("projects", stream.ReasonResync, nil),
		stream.Errored(problem.Problem{Code: problem.InvalidArgument, Message: "stream: unknown op \"gossip\""}),
	}
	got, err := json.MarshalIndent(frames, "", "  ")
	require.NoError(t, err)

	want, err := os.ReadFile("testdata/frames.json")
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}

func TestRequest_DecodesItsWireNames(t *testing.T) {
	var req stream.Request
	require.NoError(t, json.Unmarshal([]byte(`{"op":"sub","scope":"chat:c1","since":42}`), &req))
	assert.Equal(t, stream.Request{Op: stream.OpSub, Scope: "chat:c1", Since: 42}, req)
}
