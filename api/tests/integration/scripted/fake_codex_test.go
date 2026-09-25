//go:build integration && unix

package scripted_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
)

// runCodex is codex: `app-server --listen unix://…` speaks JSON-RPC over a
// websocket (the chat surface); anything else is the TUI, reporting over the
// -c hooks wiring (the terminal surface).
func runCodex(args []string) int {
	switch {
	case hasArg(args, "--version"):
		say("codex-cli 0.156.1\n")
		return 0
	case len(args) > 0 && args[0] == "app-server":
		return serveCodex(strings.TrimPrefix(argAfter(args, "--listen"), "unix://"))
	case len(args) > 1 && args[0] == "debug":
		say(`{"models":[]}` + "\n")
		return 0
	}
	cwd, _ := os.Getwd()
	c := &hookCLI{
		name: "codex", hooks: codexHooks(args), cwd: cwd, sessionFile: codexRollout, lazySession: true,
	}
	return runHookCLI(c, codexResumeID(args), positionalsAfterDashes(args))
}

func codexRollout(id string) string {
	return filepath.Join(os.Getenv("CODEX_HOME"), "sessions", "2026", "01", "01",
		"rollout-2026-01-01T00-00-00-"+id+".jsonl")
}

func codexResumeID(args []string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--" {
			return ""
		}
		if args[i] == "resume" {
			return args[i+1]
		}
	}
	return ""
}

func serveCodex(socket string) int {
	record("codex", "start", map[string]any{"argv": os.Args[1:], "serve": true})
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "unix", socket)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	upgrader := websocket.Upgrader{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		(&appServer{conn: conn, calls: map[int64]chan json.RawMessage{}}).serve()
	})}
	_ = srv.Serve(ln)
	return 0
}

// appServer is one client connection to the fake app-server.
type appServer struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	nextID  atomic.Int64
	callsMu sync.Mutex
	calls   map[int64]chan json.RawMessage
	// interrupt is closed by turn/interrupt to end the running turn.
	turnMu    sync.Mutex
	interrupt chan struct{}
}

type rpcFrame struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  json.RawMessage `json:"error,omitempty"`
}

func (a *appServer) serve() {
	for {
		_, raw, err := a.conn.ReadMessage()
		if err != nil {
			return
		}
		var f rpcFrame
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		if f.Method == "" {
			a.routeResponse(f)
			continue
		}
		if f.ID != nil {
			a.handle(f)
		}
	}
}

func (a *appServer) send(v any) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_ = a.conn.WriteJSON(v)
}

func (a *appServer) reply(id json.RawMessage, result any) {
	a.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (a *appServer) notify(method string, params map[string]any) {
	a.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// call sends a server→client request (an approval ask) and waits for the answer.
func (a *appServer) call(method string, params map[string]any) json.RawMessage {
	id := a.nextID.Add(1)
	ch := make(chan json.RawMessage, 1)
	a.callsMu.Lock()
	a.calls[id] = ch
	a.callsMu.Unlock()
	a.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	return <-ch
}

func (a *appServer) routeResponse(f rpcFrame) {
	var id int64
	if json.Unmarshal(f.ID, &id) != nil {
		return
	}
	a.callsMu.Lock()
	ch := a.calls[id]
	delete(a.calls, id)
	a.callsMu.Unlock()
	if ch != nil {
		ch <- f.Result
	}
}

func (a *appServer) handle(f rpcFrame) {
	var p map[string]any
	_ = json.Unmarshal(f.Params, &p)
	threadID, _ := p["threadId"].(string)
	record("codex", "rpc", map[string]any{"method": f.Method, "thread": threadID, "params": string(f.Params)})
	switch f.Method {
	case "thread/start":
		id := newID()
		a.reply(f.ID, map[string]any{"thread": map[string]any{"id": id}})
		a.notify("thread/started", map[string]any{"thread": map[string]any{"id": id, "path": codexRollout(id)}})
	case "thread/resume":
		if loadScript().Resume == "refuse" || !exists(codexRollout(threadID)) {
			record("codex", "refused", map[string]any{"session": threadID})
			a.send(map[string]any{"jsonrpc": "2.0", "id": f.ID, "error": map[string]any{
				"code": -32600, "message": "no rollout found for thread id " + threadID,
			}})
			return
		}
		a.reply(f.ID, map[string]any{"thread": map[string]any{"id": threadID}})
	case "turn/start":
		turnID := newID()
		a.reply(f.ID, map[string]any{"turn": map[string]any{"id": turnID, "status": "inProgress", "items": []any{}}})
		go a.turn(threadID, turnID, promptText(p))
	case "turn/interrupt":
		a.reply(f.ID, map[string]any{})
		a.turnMu.Lock()
		if a.interrupt != nil {
			close(a.interrupt)
			a.interrupt = nil
		}
		a.turnMu.Unlock()
	case "thread/compact/start":
		a.reply(f.ID, map[string]any{})
		go a.compaction(threadID)
	default:
		a.reply(f.ID, map[string]any{})
	}
}

func promptText(p map[string]any) string {
	input, _ := p["input"].([]any)
	for _, in := range input {
		if m, ok := in.(map[string]any); ok && m["type"] == "text" {
			s, _ := m["text"].(string)
			return s
		}
	}
	return ""
}

func (a *appServer) turn(threadID, turnID, prompt string) {
	s := loadScript()
	stop := make(chan struct{})
	a.turnMu.Lock()
	a.interrupt = stop
	a.turnMu.Unlock()
	record("codex", "prompt", map[string]any{"text": prompt, "session": threadID})
	a.notify("turn/started", map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "inProgress"}})
	a.notify("item/started", map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{
		"type": "userMessage", "id": newID(), "content": []any{map[string]any{"type": "text", "text": prompt}},
	}})
	for i, st := range s.Turn {
		if !a.step(threadID, turnID, i, st, stop) {
			return
		}
	}
	writeSessionFile(codexRollout(threadID))
	a.complete(threadID, turnID, "completed", s.reply())
}

func (a *appServer) complete(threadID, turnID, status, text string) {
	a.notify("thread/status/changed", map[string]any{"threadId": threadID, "status": map[string]any{"type": "idle"}})
	items := []any{}
	if text != "" {
		items = append(items, map[string]any{"type": "agentMessage", "id": newID(), "text": text})
	}
	a.notify("turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{
		"id": turnID, "status": status, "items": items,
	}})
}

// step plays one scripted step; false ends the turn early (interrupted or
// abandoned by the script).
func (a *appServer) step(threadID, turnID string, i int, st step, stop chan struct{}) bool {
	id := fmt.Sprintf("%s-%d", turnID[:8], i)
	on := func(item map[string]any) map[string]any {
		return map[string]any{"threadId": threadID, "turnId": turnID, "item": item}
	}
	switch {
	case st.Tool != "":
		a.notify("item/started", on(map[string]any{"type": "commandExecution", "id": id, "command": st.Tool, "status": "inProgress"}))
		a.notify("item/completed", on(map[string]any{
			"type": "commandExecution", "id": id, "command": st.Tool, "status": "completed", "aggregatedOutput": id, "durationMs": 1,
		}))
	case st.Permission != "":
		answer := a.call("item/commandExecution/requestApproval", map[string]any{
			"threadId": threadID, "turnId": turnID, "itemId": id, "reason": "run " + st.Permission, "command": st.Permission,
		})
		record("codex", "answer", map[string]any{"verdict": string(answer)})
	case st.Compact:
		a.notify("item/started", on(map[string]any{"type": "contextCompaction", "id": id}))
		a.notify("item/completed", on(map[string]any{"type": "contextCompaction", "id": id}))
	case st.Delta != "":
		a.notify("item/agentMessage/delta", map[string]any{"threadId": threadID, "turnId": turnID, "itemId": "msg-" + turnID, "delta": st.Delta})
	case st.Hang:
		<-stop
		a.complete(threadID, turnID, "interrupted", "")
		return false
	case st.Crash:
		record("codex", "crash", nil)
		os.Exit(3)
	case st.Drop:
		record("codex", "drop", nil)
		_ = a.conn.Close()
		hang()
	case st.Idle:
		a.notify("thread/status/changed", map[string]any{"threadId": threadID, "status": map[string]any{"type": "idle"}})
		return false
	case st.SlowMS > 0:
		pause(st.SlowMS)
	}
	return true
}

func (a *appServer) compaction(threadID string) {
	turnID := newID()
	a.notify("turn/started", map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "inProgress"}})
	on := map[string]any{"threadId": threadID, "turnId": turnID, "item": map[string]any{"type": "contextCompaction", "id": turnID}}
	a.notify("item/started", on)
	a.notify("item/completed", on)
	a.notify("turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{
		"id": turnID, "status": "completed", "items": []any{},
	}})
}
