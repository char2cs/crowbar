//go:build integration

package main

import (
	"bufio"
	"bytes"
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
	if len(args) < 3 || record(args[0], launch{RunnerID: args[1], Argv: os.Args[1:], PID: os.Getpid()}) != nil {
		return 2
	}
	var started struct {
		Params struct {
			ThreadID string `json:"threadId"`
			Turn     struct {
				ID string `json:"id"`
			} `json:"turn"`
		} `json:"params"`
	}
	if json.Unmarshal(frame(args[2], "turn_started.json"), &started) != nil {
		return 2
	}
	ids := map[string]any{"thread": map[string]any{"id": started.Params.ThreadID}, "turn": started.Params.Turn}
	out := json.NewEncoder(os.Stdout)
	lines := bufio.NewScanner(os.Stdin)
	lines.Buffer(make([]byte, 0, 1<<16), 1<<24) // a turn/start carries the whole injected context
	for lines.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(lines.Bytes(), &req) != nil || req.ID == nil {
			continue // a notification (initialized) wants no answer
		}
		if out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": ids}) != nil {
			return 0
		}
		for _, name := range serveReplies[req.Method] {
			if _, err := os.Stdout.Write(frame(args[2], name)); err != nil {
				return 0
			}
		}
	}
	return 0
}

// frame is one recorded frame as a single ndjson line, nil when unreadable.
func frame(fixtures, name string) []byte {
	raw, err := os.ReadFile(filepath.Join(fixtures, name)) //nolint:gosec // G304: the kit-named fixtures dir
	var line bytes.Buffer
	if err != nil || json.Compact(&line, raw) != nil {
		return nil
	}
	return append(line.Bytes(), '\n')
}
