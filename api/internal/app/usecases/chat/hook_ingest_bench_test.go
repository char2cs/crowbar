package chat_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

// BenchmarkIngestHookDelivery measures the relayed-hook hot path end to end:
// delivery-id dedup, runner resolution, and the observation the hook records.
// A tool_pre/tool_post pair is the commonest delivery a working agent sends.
func BenchmarkIngestHookDelivery(b *testing.B) {
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	b.Cleanup(func() { slog.SetDefault(prev) })

	f := newFixture(b)
	_, runnerID, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	if err != nil {
		b.Fatal(err)
	}
	start, _ := json.Marshal(map[string]any{"session_id": "s1"})
	if err := f.usecase.IngestHook(f.ctx, runnerID, "", "session_start", start); err != nil {
		b.Fatal(err)
	}
	f.wait()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		tool := "t" + strconv.Itoa(i)
		pre, _ := json.Marshal(map[string]any{
			"session_id": "s1", "tool_use_id": tool, "tool_name": "Read",
			"tool_input": map[string]any{"file_path": "/x"},
		})
		post, _ := json.Marshal(map[string]any{
			"session_id": "s1", "tool_use_id": tool, "tool_name": "Read", "tool_response": "ok",
		})
		if err := f.usecase.IngestHookDelivery(
			f.ctx, uuid.NewString(), runnerID, "claude", "tool_pre", pre,
		); err != nil {
			b.Fatal(err)
		}
		if err := f.usecase.IngestHookDelivery(
			f.ctx, uuid.NewString(), runnerID, "claude", "tool_post", post,
		); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	f.wait()
}
