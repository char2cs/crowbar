package agenttools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// echoWriter is the smallest ThreadWriter: it hands back an aggregate carrying the
// input's own message, so a test can post a distinctive body and then look for it.
type echoWriter struct{ id string }

func (w echoWriter) Open(
	_ context.Context,
	in reviewthread.OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{
		ID:        w.id,
		WsID:      in.WsID,
		FilePath:  in.FilePath,
		StartLine: in.StartLine,
		EndLine:   in.EndLine,
		Side:      in.Side,
		Status:    domain.ReviewThreadStatusOpen,
		Messages: []domain.ReviewMessage{{
			ID: in.MessageID, Author: in.Author, IsAgent: in.IsAgent, Body: in.Body, CreatedAt: now,
		}},
	}, nil
}

func (w echoWriter) Reply(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ bool,
	_ string,
	_ time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (w echoWriter) Resolve(
	_ context.Context,
	_ string,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

// cachedDump renders every cached value with all its fields, including unexported
// and nested ones, so a search over it sees whatever the map is actually holding.
func cachedDump(i *Idempotency) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var b strings.Builder
	for ref, v := range i.opened {
		fmt.Fprintf(&b, "%#v => %#v\n", ref, v)
	}
	return b.String()
}

// TestIdempotency_DoesNotRetainTheFindingBody is a retention guard, not a behaviour
// test. The dedup map is unbounded and lives for the life of the process, so anything
// it holds is held forever; the only reason to hold anything is the retry reply, which
// consumes the anchor alone. Caching the aggregate would keep arbitrary user-authored
// review markdown resident with no consumer.
//
// It searches a full field dump rather than asserting on a struct shape, so it fails
// if the cached value is ever widened back to a body-bearing type — which a
// compile-time field check would not catch.
func TestIdempotency_DoesNotRetainTheFindingBody(t *testing.T) {
	const body = "UNIQUE-FINDING-BODY-e3b0c442"
	const author = "UNIQUE-AUTHOR-9f86d081"
	idem := NewIdempotency()

	out, err := idem.openOnce(
		context.Background(),
		echoWriter{id: "t1"},
		"a-key",
		reviewthread.OpenInput{
			ID: "ignored", WsID: "ws-a", FilePath: "src/auth.go",
			StartLine: 42, EndLine: 44, Side: domain.ReviewSideRight,
			MessageID: "m1", Author: author, IsAgent: true, Body: body,
		},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	require.True(t, out.created)
	// The write's own return value DOES carry the finding — it is the broadcast
	// payload. Only the cache is narrowed.
	require.Equal(t, body, out.fresh.Messages[0].Body)

	dump := cachedDump(idem)
	require.NotContains(t, dump, body, "the dedup map must not retain the finding body: %s", dump)
	require.NotContains(t, dump, author, "the dedup map must not retain the message author: %s", dump)

	// What it must retain: exactly enough to answer a retry with the STORED anchor.
	require.Contains(t, dump, "t1")
	require.Contains(t, dump, "src/auth.go")
	require.Len(t, idem.opened, 1)
}

// The narrowed cache must still answer a retry with the stored anchor rather than the
// retry's own arguments, and must not report a write it did not perform.
func TestIdempotency_ARetryIsAnsweredFromTheCachedAnchor(t *testing.T) {
	idem := NewIdempotency()
	in := reviewthread.OpenInput{
		ID: "ignored", WsID: "ws-a", FilePath: "src/auth.go",
		StartLine: 42, EndLine: 44, Side: domain.ReviewSideRight,
		MessageID: "m1", Body: "first",
	}

	first, err := idem.openOnce(context.Background(), echoWriter{id: "t1"}, "k", in, time.Unix(1, 0).UTC())
	require.NoError(t, err)
	require.True(t, first.created)

	moved := in
	moved.StartLine, moved.EndLine, moved.Body = 47, 48, "second"
	retry, err := idem.openOnce(context.Background(), echoWriter{id: "t2"}, "k", moved, time.Unix(2, 0).UTC())
	require.NoError(t, err)

	require.False(t, retry.created, "a dedup hit performed no write")
	require.Equal(t, "t1", retry.stored.ID)
	require.Equal(t, 42, retry.stored.StartLine)
	require.Equal(t, 44, retry.stored.EndLine)
	require.Equal(t, domain.ReviewSideRight, retry.stored.Side)
	// created is false, so there is no fresh aggregate to announce.
	require.Zero(t, retry.fresh.ID)
	require.Empty(t, retry.fresh.Messages)
}

// An unkeyed call is not remembered at all, so nothing of it is retained.
func TestIdempotency_AnUnkeyedCallCachesNothing(t *testing.T) {
	idem := NewIdempotency()

	out, err := idem.openOnce(
		context.Background(),
		echoWriter{id: "t1"},
		"",
		reviewthread.OpenInput{WsID: "ws-a", MessageID: "m1", Body: "no key"},
		time.Unix(1, 0).UTC(),
	)
	require.NoError(t, err)
	require.True(t, out.created)
	require.Empty(t, idem.opened, "no key means no entry to keep")
}
