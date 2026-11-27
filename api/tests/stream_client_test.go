//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type streamClient struct {
	conn *websocket.Conn
}

func dialStream(t *testing.T, h *harness) *streamClient {
	t.Helper()
	return &streamClient{conn: h.dial("/v0/stream")}
}

func (c *streamClient) Sub(t *testing.T, scope string, since int64) {
	t.Helper()
	require.NoError(t, c.conn.WriteJSON(stream.Request{Op: stream.OpSub, Scope: scope, Since: since}))
}

func (c *streamClient) Unsub(t *testing.T, scope string) {
	t.Helper()
	require.NoError(t, c.conn.WriteJSON(stream.Request{Op: stream.OpUnsub, Scope: scope}))
}

func (c *streamClient) Next(t *testing.T, match func(stream.Frame) bool) stream.Frame {
	t.Helper()
	var got stream.Frame
	readUntil(t, c.conn, func(raw map[string]any) bool {
		encoded, err := json.Marshal(raw)
		require.NoError(t, err)
		got = stream.Frame{}
		require.NoError(t, json.Unmarshal(encoded, &got))
		return match(got)
	})
	return got
}

func (c *streamClient) Hold(t *testing.T, scope string) {
	t.Helper()
	c.Sub(t, scope, 0)
	c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameSynced && f.Scope == scope })
}

func (c *streamClient) Signal(t *testing.T, scope, kind string, match func(map[string]any) bool) map[string]any {
	t.Helper()
	var data map[string]any
	c.Next(t, func(f stream.Frame) bool {
		if f.T != stream.FrameSignal || f.Scope != scope || f.Kind != kind {
			return false
		}
		data = map[string]any{}
		require.NoError(t, json.Unmarshal(f.Data, &data))
		return match(data)
	})
	return data
}

func operationIn(t *testing.T, f stream.Frame) domain.Operation {
	t.Helper()
	var op domain.Operation
	require.NoError(t, json.Unmarshal(f.Data, &op))
	return op
}

func isOperationFrame(f stream.Frame, id string) bool {
	return f.T == stream.FrameEntity && f.Kind == string(stream.KindOperation) && f.ID == id
}

func awaitOperation(
	t *testing.T,
	h *harness,
	id string,
	done func(domain.Operation) bool,
) domain.Operation {
	t.Helper()
	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	var got domain.Operation
	c.Next(t, func(f stream.Frame) bool {
		if !isOperationFrame(f, id) {
			return false
		}
		got = operationIn(t, f)
		return done(got)
	})
	require.NoError(t, c.conn.Close())
	return got
}

type entities struct {
	c    *streamClient
	kind stream.EntityKind
}

func watchEntities(t *testing.T, h *harness, kind stream.EntityKind) *entities {
	t.Helper()
	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	return &entities{c: c, kind: kind}
}

func (e *entities) read(t *testing.T, match func(map[string]any) bool) map[string]any {
	t.Helper()
	var data map[string]any
	e.c.Next(t, func(f stream.Frame) bool {
		if f.T != stream.FrameEntity || f.Kind != string(e.kind) {
			return false
		}
		data = map[string]any{"id": f.ID, "gone": true}
		if !f.Gone {
			data = map[string]any{}
			require.NoError(t, json.Unmarshal(f.Data, &data))
		}
		return match(data)
	})
	return data
}

const frameBound = 30 * time.Second

func frameDeadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), frameBound)
	t.Cleanup(cancel)
	return ctx
}

type feedRow struct {
	Kind string
	ID   string
	Gone bool
	Data map[string]any
}

type sidebarFeed struct {
	t      *testing.T
	mu     sync.Mutex
	seen   []feedRow
	live   int
	synced bool
	ping   chan struct{}
	held   chan struct{}
}

func recordSidebar(t *testing.T, h *harness) *sidebarFeed {
	t.Helper()
	c := dialStream(t, h)
	c.Sub(t, "projects", 0)
	f := &sidebarFeed{t: t, ping: make(chan struct{}, 1), held: make(chan struct{})}
	go f.read(c.conn)
	select {
	case <-f.held:
	case <-frameDeadline(t).Done():
		t.Fatal("the projects scope never finished its snapshot")
	}
	return f
}

func (f *sidebarFeed) read(conn *websocket.Conn) {
	for {
		var fr stream.Frame
		if conn.ReadJSON(&fr) != nil {
			return
		}
		if fr.T == stream.FrameSynced && !f.synced {
			f.mu.Lock()
			f.live, f.synced = len(f.seen), true
			f.mu.Unlock()
			close(f.held)
		}
		if fr.T != stream.FrameEntity {
			continue
		}
		row := feedRow{Kind: fr.Kind, ID: fr.ID, Gone: fr.Gone}
		if !fr.Gone && json.Unmarshal(fr.Data, &row.Data) != nil {
			continue
		}
		f.mu.Lock()
		f.seen = append(f.seen, row)
		f.mu.Unlock()
		select {
		case f.ping <- struct{}{}:
		default:
		}
	}
}

func (f *sidebarFeed) rows() []feedRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedRow(nil), f.seen...)
}

func (f *sidebarFeed) liveRows() []feedRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedRow(nil), f.seen[f.live:]...)
}

func (f *sidebarFeed) await(what string, match func(i int, r feedRow) bool) feedRow {
	f.t.Helper()
	deadline := frameDeadline(f.t)
	for {
		for i, r := range f.rows() {
			if match(i, r) {
				return r
			}
		}
		select {
		case <-f.ping:
		case <-deadline.Done():
			f.t.Fatalf("timed out waiting for %s; rows seen: %v", what, f.rows())
			return feedRow{}
		}
	}
}

func (f *sidebarFeed) awaitChat(chatID, what string, match func(chat map[string]any) bool) map[string]any {
	f.t.Helper()
	return f.await(fmt.Sprintf("chat %s %s", chatID, what), func(_ int, r feedRow) bool {
		return r.Kind == string(stream.KindChatEntity) && r.ID == chatID && !r.Gone && match(r.Data)
	}).Data
}

func (f *sidebarFeed) awaitNewChat(match func(chat map[string]any) bool) map[string]any {
	f.t.Helper()
	isChat := func(r feedRow) bool { return r.Kind == string(stream.KindChatEntity) && !r.Gone }
	return f.await("a new chat with its workspace", func(i int, r feedRow) bool {
		snapshot, ok := f.snapshot()
		if !ok || i < len(snapshot) || !isChat(r) || !match(r.Data) {
			return false
		}
		for _, held := range snapshot {
			if isChat(held) && held.ID == r.ID {
				return false
			}
		}
		return true
	}).Data
}

func (f *sidebarFeed) snapshot() ([]feedRow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedRow(nil), f.seen[:f.live]...), f.synced
}

func (f *sidebarFeed) awaitWorkspace(what string, match func(ws map[string]any) bool) map[string]any {
	f.t.Helper()
	return f.await("a workspace "+what, func(_ int, r feedRow) bool {
		return r.Kind == string(stream.KindWorkspace) && !r.Gone && match(r.Data)
	}).Data
}

func hasWorkspace(chat map[string]any) bool { return chat["workspaceId"] != "" }

func (f *sidebarFeed) awaitListed(chatID string) map[string]any {
	f.t.Helper()
	return f.awaitChat(chatID, "listed", func(map[string]any) bool { return true })
}

func (f *sidebarFeed) awaitGone(kind stream.EntityKind, id string) {
	f.t.Helper()
	f.await(fmt.Sprintf("%s %s gone", kind, id), func(_ int, r feedRow) bool {
		return r.Kind == string(kind) && r.ID == id && r.Gone
	})
}

func (f *sidebarFeed) awaitConversation(chatID, sessionID string) {
	f.t.Helper()
	f.awaitChat(chatID, "bound to session "+sessionID, func(chat map[string]any) bool {
		conversation, _ := chat["conversation"].(map[string]any)
		return conversation["sessionId"] == sessionID && conversation["state"] == "bound"
	})
}

func (f *sidebarFeed) awaitTurnStart(chatID string) {
	f.t.Helper()
	f.awaitChat(chatID, "working", func(chat map[string]any) bool { return chat["working"] == true })
}

func (f *sidebarFeed) awaitTurnEnd(chatID string) {
	f.t.Helper()
	f.awaitAfter(chatID, "idle after a turn",
		func(chat map[string]any) bool { return chat["working"] == true },
		func(chat map[string]any) bool { return chat["working"] != true })
}

func (f *sidebarFeed) awaitRunnerGone(chatID, runnerID string) {
	f.t.Helper()
	f.awaitAfter(chatID, "rid of runner "+runnerID,
		func(chat map[string]any) bool { return chat["liveRunnerId"] == runnerID },
		func(chat map[string]any) bool { return chat["liveRunnerId"] != runnerID })
}

func (f *sidebarFeed) awaitAfter(chatID, what string, first, then func(map[string]any) bool) {
	f.t.Helper()
	isChat := func(r feedRow) bool { return r.Kind == string(stream.KindChatEntity) && r.ID == chatID && !r.Gone }
	f.await(fmt.Sprintf("chat %s %s", chatID, what), func(i int, r feedRow) bool {
		if !isChat(r) || !then(r.Data) {
			return false
		}
		for _, before := range f.rows()[:i] {
			if isChat(before) && first(before.Data) {
				return true
			}
		}
		return false
	})
}

func dialStreamAs(t *testing.T, h *pairedHarness, token string) (*streamClient, int) {
	t.Helper()
	transport, ok := h.tls.Client().Transport.(*http.Transport)
	require.True(t, ok, "the TLS test server's client has an *http.Transport")
	dialer := websocket.Dialer{TLSClientConfig: transport.TLSClientConfig}
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	conn, resp, err := dialer.Dial("wss"+strings.TrimPrefix(h.tls.URL, "https")+"/v0/stream", header)
	status := 0
	if resp != nil {
		status = resp.StatusCode
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, status
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &streamClient{conn: conn}, status
}
