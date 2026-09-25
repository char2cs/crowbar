//go:build integration && unix

package scripted_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// liveReply is what the stand-in model answers every request with.
const liveReply = "LIVE-MODEL-REPLY"

// modelServer stands in for the Anthropic Messages and OpenAI Responses APIs,
// so the REAL claude and codex CLIs run whole turns with no credentials. hold
// parks every request until released, to catch a CLI mid-turn.
type modelServer struct {
	*httptest.Server
	mu   sync.Mutex
	hold chan struct{}
	// replies numbers each reply: a real API never repeats a message id, and
	// the CLIs key their items by it.
	replies atomic.Int64
}

func newModelServer(t *testing.T) *modelServer {
	t.Helper()
	m := &modelServer{}
	m.Server = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(func() {
		m.release()
		m.Close()
	})
	return m
}

// holdTurns parks every model request from now on until release.
func (m *modelServer) holdTurns() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hold = make(chan struct{})
}

func (m *modelServer) release() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hold != nil {
		close(m.hold)
		m.hold = nil
	}
}

func (m *modelServer) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method != http.MethodPost:
		writeJSON(w, map[string]any{"data": []any{}, "models": []any{}})
	case strings.Contains(r.URL.Path, "count_tokens"):
		writeJSON(w, map[string]any{"input_tokens": 10})
	case strings.Contains(r.URL.Path, "/messages"), strings.Contains(r.URL.Path, "/responses"):
		m.mu.Lock()
		hold := m.hold
		m.mu.Unlock()
		if hold != nil {
			select {
			case <-hold:
			case <-r.Context().Done():
				return
			}
		}
		id := fmt.Sprintf("live_%d", m.replies.Add(1))
		if strings.Contains(r.URL.Path, "/responses") {
			openAIStream(w, id)
			return
		}
		anthropicReply(w, body, id)
	default:
		writeJSON(w, map[string]any{})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func sse(w http.ResponseWriter, name string, data any) {
	raw, _ := json.Marshal(data)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, raw)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func anthropicReply(w http.ResponseWriter, body []byte, id string) {
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	_ = json.Unmarshal(body, &req)
	if !req.Stream {
		writeJSON(w, map[string]any{
			"id": "msg_"+id, "type": "message", "role": "assistant", "model": req.Model,
			"content": []any{map[string]any{"type": "text", "text": liveReply}}, "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 3},
		})
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	sse(w, "message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": "msg_"+id, "type": "message", "role": "assistant", "model": req.Model, "content": []any{},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 1},
	}})
	sse(w, "content_block_start", map[string]any{"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""}})
	sse(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": 0,
		"delta": map[string]any{"type": "text_delta", "text": liveReply}})
	sse(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": 0})
	sse(w, "message_delta", map[string]any{"type": "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 3}})
	sse(w, "message_stop", map[string]any{"type": "message_stop"})
}

func openAIStream(w http.ResponseWriter, id string) {
	w.Header().Set("content-type", "text/event-stream")
	base := map[string]any{"id": "resp_"+id, "object": "response", "status": "in_progress", "model": "gpt-live", "output": []any{}}
	item := map[string]any{"id": "msg_"+id, "type": "message", "role": "assistant", "status": "in_progress", "content": []any{}}
	sse(w, "response.created", map[string]any{"type": "response.created", "response": base})
	sse(w, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 0, "item": item})
	sse(w, "response.content_part.added", map[string]any{"type": "response.content_part.added", "item_id": "msg_"+id,
		"output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
	sse(w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "item_id": "msg_"+id,
		"output_index": 0, "content_index": 0, "delta": liveReply})
	done := map[string]any{"id": "msg_"+id, "type": "message", "role": "assistant", "status": "completed",
		"content": []any{map[string]any{"type": "output_text", "text": liveReply, "annotations": []any{}}}}
	sse(w, "response.output_item.done", map[string]any{"type": "response.output_item.done", "output_index": 0, "item": done})
	final := map[string]any{"id": "resp_"+id, "object": "response", "status": "completed", "model": "gpt-live",
		"output": []any{done}, "usage": map[string]any{"input_tokens": 10, "output_tokens": 3, "total_tokens": 13,
			"input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens_details": map[string]any{"reasoning_tokens": 0}}}
	sse(w, "response.completed", map[string]any{"type": "response.completed", "response": final})
}
