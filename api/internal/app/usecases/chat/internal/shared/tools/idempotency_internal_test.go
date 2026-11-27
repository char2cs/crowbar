package tools

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
	_ reviewthread.ReplyInput,
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

func cachedDump(i *Idempotency) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var b strings.Builder
	for ref, v := range i.opened {
		fmt.Fprintf(&b, "%#v => %#v\n", ref, v)
	}
	return b.String()
}

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
	require.Equal(t, body, out.fresh.Messages[0].Body)

	dump := cachedDump(idem)
	require.NotContains(t, dump, body, "the dedup map must not retain the finding body: %s", dump)
	require.NotContains(t, dump, author, "the dedup map must not retain the message author: %s", dump)

	require.Contains(t, dump, "t1")
	require.Contains(t, dump, "src/auth.go")
	require.Len(t, idem.opened, 1)
}

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
	require.Zero(t, retry.fresh.ID)
	require.Empty(t, retry.fresh.Messages)
}

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
