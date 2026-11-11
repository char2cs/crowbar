package ingress

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestRecorder_AToolsPayloadsAreStoredBeforeItsRows(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{Kind: engineagents.HookUserPrompt, Message: "go"})
	require.NoError(t, err)

	_, _, err = f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPost,
		Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash", Input: []byte(`{"command":"ls"}`), Result: []byte("a.go")},
	})

	require.NoError(t, err)
	rows := f.rows(t)
	call := bodyOf[ledger.ToolCall](t, rows[len(rows)-1])
	assert.NotEmpty(t, call.InputRef)
	assert.NotEmpty(t, call.ResultRef)
	refs, err := f.ledger.BlobRefs(t.Context(), testChat)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{call.InputRef, call.ResultRef}, refs)
}

func TestRecorder_AFailedToolsWholeErrorIsItsResult(t *testing.T) {
	f := newRecorderFixture(t, "claude")
	whole := strings.Repeat("é", 40_000)

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolFail, Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash", Error: whole},
	})

	require.NoError(t, err)
	rows := f.rows(t)
	call := bodyOf[ledger.ToolCall](t, rows[len(rows)-1])
	assert.Less(t, len(call.Error), len(whole), "its row keeps a caption")
	got, err := f.blobs.Get(call.ResultRef)
	require.NoError(t, err)
	assert.Equal(t, whole, string(got))
}

func TestRecorder_AToolIsRecordedWithoutThePayloadsItCannotStore(t *testing.T) {
	testutil.RequirePermissionEnforcement(t)
	f := newRecorderFixture(t, "claude")
	require.NoError(t, os.Chmod(f.blobDir, 0o500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(f.blobDir, 0o700)) })

	_, _, err := f.dispatch(t, engineagents.CanonicalEvent{
		Kind: engineagents.HookToolPost,
		Tool: &engineagents.ToolEvent{ID: "t1", Name: "Bash", Input: []byte("args"), Result: []byte("out")},
	})

	require.NoError(t, err)
	rows := f.rows(t)
	call := bodyOf[ledger.ToolCall](t, rows[len(rows)-1])
	assert.Equal(t, "Bash", call.Name)
	assert.Empty(t, call.InputRef)
	assert.Empty(t, call.ResultRef)
}

func TestMap_AFailedToolsRowKeepsACaptionOfItsError(t *testing.T) {
	c := liveChatRows(t)
	ev := toolEvent(engineagents.HookToolFail, "t1", "Bash")
	ev.Tool.Error = strings.Repeat("é", 40_000)

	rows, _ := c.mapped(ev, hooksSource())

	caption := bodyOf[ledger.ToolCall](t, rows[0]).Error
	assert.LessOrEqual(t, len(caption), 2<<10)
	assert.True(t, utf8.ValidString(caption), "a character straddling the cut is dropped whole")
}
