//go:build integration

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
)

var serveReplies = map[string][]string{
	"thread/start":  {"thread_started.json"},
	"thread/resume": {"thread_started.json"},
	"turn/start": {
		"turn_started.json", "item_started.json", "item_completed.json",
		"item_agentMessage_delta.json", "turn_completed.json",
	},
}

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
	replies := serveReplies
	if len(args) > 3 {
		turn := append(append([]string{"turn_started.json"}, args[3:]...), "turn_completed.json")
		replies = map[string][]string{"thread/start": serveReplies["thread/start"], "thread/resume": serveReplies["thread/resume"], "turn/start": turn}
	}
	out := json.NewEncoder(os.Stdout)
	lines := bufio.NewScanner(os.Stdin)
	lines.Buffer(make([]byte, 0, 1<<16), 1<<24)
	for lines.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(lines.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		if out.Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": ids}) != nil {
			return 0
		}
		for _, name := range replies[req.Method] {
			if _, err := os.Stdout.Write(respoken(frame(args[2], name), started.Params.ThreadID, started.Params.Turn.ID)); err != nil {
				return 0
			}
		}
	}
	return 0
}

func respoken(line []byte, thread, turn string) []byte {
	var msg map[string]any
	if json.Unmarshal(line, &msg) != nil {
		return line
	}
	params, ok := msg["params"].(map[string]any)
	if !ok {
		return line
	}
	if _, ok := params["threadId"]; ok {
		params["threadId"] = thread
	}
	if _, ok := params["turnId"]; ok {
		params["turnId"] = turn
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return line
	}
	return append(out, '\n')
}

func frame(fixtures, name string) []byte {
	raw, err := os.ReadFile(filepath.Join(fixtures, name)) //nolint:gosec // G304: the kit-named fixtures dir
	var line bytes.Buffer
	if err != nil || json.Compact(&line, raw) != nil {
		return nil
	}
	return append(line.Bytes(), '\n')
}
