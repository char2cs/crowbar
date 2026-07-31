package agenttools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ── the write-time body cap ─────────────────────────────────────────
// maxMessageBodyChars and maxTurnBodyChars bound what is RENDERED BACK and
// nothing at all about what is stored. maxWrittenBodyChars is the input bound,
// and it matters beyond the row it is written to: replying emits the whole
// thread aggregate as the event payload, so every body already on a thread is
// re-serialised by every later message on it.

func TestPostReviewComment_RefusesAnOverlongBodyAndNamesTheLimit(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())
	over := agenttools.MaxWrittenBodyCharsForTest + 1

	_, err := f.post(fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":%q}`,
		longBody(over)))

	require.Error(t, err)
	// The model's only recovery is to shorten and retry, which it cannot do
	// against an error that merely says "too long".
	require.Contains(t, err.Error(), fmt.Sprint(over))
	require.Contains(t, err.Error(), fmt.Sprint(agenttools.MaxWrittenBodyCharsForTest))
	require.Empty(t, f.writer.opens, "a refused body must never reach the store")
	require.Empty(t, f.broadcast.frames)
}

func TestPostReviewComment_AcceptsABodyExactlyAtTheLimit(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())

	_, err := f.post(fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":%q}`,
		longBody(agenttools.MaxWrittenBodyCharsForTest)))

	require.NoError(t, err, "the limit is inclusive")
	require.Len(t, f.writer.opens, 1)
}

// RUNES, not bytes — the same rule truncateBody counts by. A body of exactly the
// limit in multi-byte characters is three times the limit in bytes and must
// still be accepted, or the cap silently becomes three times stricter for
// anything that is not ASCII.
func TestPostReviewComment_CountsTheBodyInRunesNotBytes(t *testing.T) {
	f := postOn(t, "ws-a", authHunk())
	body := strings.Repeat("é", agenttools.MaxWrittenBodyCharsForTest)
	require.Greater(t, len(body), agenttools.MaxWrittenBodyCharsForTest, "the fixture must be multi-byte")

	_, err := f.post(fmt.Sprintf(
		`{"filePath":"src/auth.go","startLine":42,"endLine":44,"side":"right","body":%q}`, body))

	require.NoError(t, err)
	require.Len(t, f.writer.opens, 1)
}

func TestReplyToReviewThread_RefusesAnOverlongBodyAndNamesTheLimit(t *testing.T) {
	spy := &spyThreads{thread: domain.ReviewThread{ID: "t1", WsID: "ws-a"}}
	ts := reviewToolsOn(t, spy)
	over := agenttools.MaxWrittenBodyCharsForTest + 1

	_, err := ts.Call(context.Background(), "reply_to_review_thread",
		json.RawMessage(fmt.Sprintf(`{"threadId":"t1","body":%q}`, longBody(over))))

	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprint(over))
	require.Contains(t, err.Error(), fmt.Sprint(agenttools.MaxWrittenBodyCharsForTest))
	require.Empty(t, spy.replied, "a refused body must never reach the store")
}
