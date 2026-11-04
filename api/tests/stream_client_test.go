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

// streamClient is one /v0/stream connection as a test drives it.
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

// Next reads frames until match holds, under readUntil's failure bound.
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

// Hold subs scope and reads past its synced: what follows is live.
func (c *streamClient) Hold(t *testing.T, scope string) {
	t.Helper()
	c.Sub(t, scope, 0)
	c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameSynced && f.Scope == scope })
}

// Signal reads until a signal of kind on scope whose data match accepts, and
// answers that data.
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

// awaitOperation holds the projects scope and reads until done holds for
// operation id. The scope's snapshot carries the operation's current row, so a
// change that happened before the dial is never missed.
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

// entities is one kind's rows on the projects scope: its snapshot, then each
// live change. A gone row reads as {"id": id, "gone": true}.
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

// read answers the first row of the kind that match accepts.
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

// frameBound is how long a test waits for a frame the daemon owes it: one that
// never comes fails the test by name instead of hanging the package.
const frameBound = 30 * time.Second

// frameDeadline is t's context, ended frameBound from now.
func frameDeadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), frameBound)
	t.Cleanup(cancel)
	return ctx
}

// feedRow is one projects-scope entity frame; a gone row carries no data.
type feedRow struct {
	Kind string
	ID   string
	Gone bool
	Data map[string]any
}

// sidebarFeed records every entity frame the projects scope sends, snapshot
// first, so a test can await a row that landed before it asked.
type sidebarFeed struct {
	t    *testing.T
	mu   sync.Mutex
	seen []feedRow
	// live is where the snapshot ended in seen, once synced.
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
	// Held from here on: a change made after this returns is a live row.
	select {
	case <-f.held:
	case <-frameDeadline(t).Done():
		t.Fatal("the projects scope never finished its snapshot")
	}
	return f
}

// read records frames until the connection closes at test cleanup.
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

// rows copies the rows recorded so far.
func (f *sidebarFeed) rows() []feedRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedRow(nil), f.seen...)
}

// liveRows copies the rows recorded after the snapshot.
func (f *sidebarFeed) liveRows() []feedRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedRow(nil), f.seen[f.live:]...)
}

// await blocks until match accepts some recorded row, scanning in arrival
// order with each row's index; it re-scans when a row lands.
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

// awaitChat answers the first row of chatID that match accepts.
func (f *sidebarFeed) awaitChat(chatID, what string, match func(chat map[string]any) bool) map[string]any {
	f.t.Helper()
	return f.await(fmt.Sprintf("chat %s %s", chatID, what), func(_ int, r feedRow) bool {
		return r.Kind == string(stream.KindChatEntity) && r.ID == chatID && !r.Gone && match(r.Data)
	}).Data
}

// awaitNewChat answers the first live row of a chat the snapshot did not
// hold that match accepts.
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

// snapshot copies the snapshot's rows, once it has fully landed.
func (f *sidebarFeed) snapshot() ([]feedRow, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]feedRow(nil), f.seen[:f.live]...), f.synced
}

// awaitWorkspace answers the first workspace row that match accepts.
func (f *sidebarFeed) awaitWorkspace(what string, match func(ws map[string]any) bool) map[string]any {
	f.t.Helper()
	return f.await("a workspace "+what, func(_ int, r feedRow) bool {
		return r.Kind == string(stream.KindWorkspace) && !r.Gone && match(r.Data)
	}).Data
}

// hasWorkspace is a chat row that names its workspace.
func hasWorkspace(chat map[string]any) bool { return chat["workspaceId"] != "" }

// awaitListed waits for chatID's first row.
func (f *sidebarFeed) awaitListed(chatID string) map[string]any {
	f.t.Helper()
	return f.awaitChat(chatID, "listed", func(map[string]any) bool { return true })
}

// awaitGone waits for the gone row of kind/id.
func (f *sidebarFeed) awaitGone(kind stream.EntityKind, id string) {
	f.t.Helper()
	f.await(fmt.Sprintf("%s %s gone", kind, id), func(_ int, r feedRow) bool {
		return r.Kind == string(kind) && r.ID == id && r.Gone
	})
}

// awaitConversation waits for chatID's row bound to sessionID.
func (f *sidebarFeed) awaitConversation(chatID, sessionID string) {
	f.t.Helper()
	f.awaitChat(chatID, "bound to session "+sessionID, func(chat map[string]any) bool {
		conversation, _ := chat["conversation"].(map[string]any)
		return conversation["sessionId"] == sessionID && conversation["state"] == "bound"
	})
}

// awaitTurnStart waits for chatID's row that shows a turn running.
func (f *sidebarFeed) awaitTurnStart(chatID string) {
	f.t.Helper()
	f.awaitChat(chatID, "working", func(chat map[string]any) bool { return chat["working"] == true })
}

// awaitTurnEnd waits for chatID's idle row after one that showed a turn running.
func (f *sidebarFeed) awaitTurnEnd(chatID string) {
	f.t.Helper()
	f.awaitAfter(chatID, "idle after a turn",
		func(chat map[string]any) bool { return chat["working"] == true },
		func(chat map[string]any) bool { return chat["working"] != true })
}

// awaitRunnerGone waits for the chat runnerID ran on to name it no longer.
func (f *sidebarFeed) awaitRunnerGone(chatID, runnerID string) {
	f.t.Helper()
	f.awaitAfter(chatID, "rid of runner "+runnerID,
		func(chat map[string]any) bool { return chat["liveRunnerId"] == runnerID },
		func(chat map[string]any) bool { return chat["liveRunnerId"] != runnerID })
}

// awaitAfter waits for chatID's row that then accepts, recorded after one that
// first accepted.
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

// dialStreamAs opens /v0/stream over h's TLS listener, carrying token as its
// bearer when it is not empty. It returns the upgrade's status.
func dialStreamAs(t *testing.T, h *pairedHarness, token string) (*streamClient, int) {
	t.Helper()
	tlsConfig := h.tls.Client().Transport.(*http.Transport).TLSClientConfig
	dialer := websocket.Dialer{TLSClientConfig: tlsConfig}
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
