//go:build integration

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

// serveReplies are the recorded notifications each request is followed by: one
// live codex app-server session's turn, replayed verbatim from the fixtures dir.
var serveReplies = map[string][]string{
	"thread/start":  {"thread_started.json"},
	"thread/resume": {"thread_started.json"},
	"turn/start": {
		"turn_started.json", "item_started.json", "item_completed.json",
		"item_agentMessage_delta.json", "turn_completed.json",
	},
}

// serveApp is `app-server <capture dir> <runner id> <fixtures dir>`: it records
// its launch, answers each request with the ids the recording carries, and
// follows it with that request's recorded notifications, until stdin closes.
func serveApp(args []string) int {
	if len(args) < 3 {
		return 2
	}
	dir, fixtures := args[0], args[2]
	if record(dir, launch{RunnerID: args[1], Argv: os.Args[1:], PID: os.Getpid()}) != nil {
		return 2
	}
	thread, turn := recordedIDs(fixtures)
	out := json.NewEncoder(os.Stdout)
	lines := bufio.NewScanner(os.Stdin)
	lines.Buffer(make([]byte, 0, 1<<16), 1<<24)
	for lines.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(lines.Bytes(), &req) != nil || req.ID == nil {
			continue // a notification (initialized) wants no answer
		}
		result := map[string]any{"thread": map[string]any{"id": thread}, "turn": map[string]any{"id": turn}}
		if out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}) != nil {
			return 0
		}
		for _, name := range serveReplies[req.Method] {
			if replay(out, filepath.Join(fixtures, name)) != nil {
				return 0
			}
		}
	}
	return 0
}

// recordedIDs are the thread and turn the recorded session ran as.
func recordedIDs(fixtures string) (thread, turn string) {
	var frame struct {
		Params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		} `json:"params"`
	}
	raw, err := os.ReadFile(filepath.Join(fixtures, "turn_started.json")) //nolint:gosec // G304: the kit-named fixtures dir
	if err == nil && json.Unmarshal(raw, &frame) == nil {
		return frame.Params.ThreadID, frame.Params.Turn.ID
	}
	return "", ""
}

// replay writes one recorded frame as a single ndjson line.
func replay(out *json.Encoder, path string) error {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the kit-named fixtures dir
	if err != nil {
		return err
	}
	var frame json.RawMessage
	if err := json.Unmarshal(raw, &frame); err != nil {
		return err
	}
	return out.Encode(frame)
}
