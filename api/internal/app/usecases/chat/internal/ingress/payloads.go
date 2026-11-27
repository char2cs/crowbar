package ingress

import (
	"context"
	"log/slog"
	"unicode/utf8"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (r *Recorder) putBlobs(ctx context.Context, ev engineagents.CanonicalEvent) BlobRefs {
	if ev.Tool == nil {
		return BlobRefs{}
	}
	return BlobRefs{Input: r.putBlob(ctx, "input", ev.Tool.Input), Result: r.putBlob(ctx, "result", resultPayload(ev.Tool))}
}

func (r *Recorder) putBlob(ctx context.Context, payload string, data []byte) string {
	if len(data) == 0 {
		return ""
	}
	ref, err := r.blobs.Put(data)
	if err != nil {
		slog.WarnContext(ctx, "agent: hook: tool payload not stored", "payload", payload, "err", err)
		return ""
	}
	return ref
}

const maxCaption = 2 << 10

func caption(s string) string { return cut(s, maxCaption) }

func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func resultPayload(tool *engineagents.ToolEvent) []byte {
	if len(tool.Result) == 0 && len(tool.Error) > maxCaption {
		return []byte(tool.Error)
	}
	return tool.Result
}
