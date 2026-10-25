package tools_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// desktopPeer is a fake desktop app: it reports its tabs to the real bridge,
// records every request it is asked to run, and answers each with reply.
type desktopPeer struct {
	mu       sync.Mutex
	requests []map[string]any
	reply    string
	session  *browserbridge.Session
}

func (d *desktopPeer) Send(raw []byte) error {
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	d.mu.Lock()
	d.requests = append(d.requests, f)
	d.mu.Unlock()
	resp, _ := json.Marshal(map[string]any{
		"type": "response", "id": f["id"], "ok": true, "result": json.RawMessage(d.reply),
	})
	d.session.Receive(resp)
	return nil
}

func (d *desktopPeer) ran() []map[string]any {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]map[string]any(nil), d.requests...)
}

func (d *desktopPeer) reportTabs(tabs ...[2]string) {
	items := []map[string]any{}
	for _, tab := range tabs {
		items = append(items, map[string]any{"id": tab[0], "chatId": tab[1], "url": "https://x.test", "title": "T", "loading": false})
	}
	raw, _ := json.Marshal(map[string]any{"type": "tabs", "tabs": items})
	d.session.Receive(raw)
}

type browserWorld struct {
	peer *desktopPeer
	a, b *tools.ToolSet // two agents, in chat-a and chat-b
}

func newBrowserWorld(t *testing.T) browserWorld {
	t.Helper()
	bridge := browserbridge.New()
	peer := &desktopPeer{reply: `"ok"`}
	peer.session = bridge.Attach(peer)
	resA, minterA := resolverOnChat(t, "chat-a")
	resB, minterB := resolverOnChat(t, "chat-b")
	return browserWorld{
		peer: peer,
		a:    tools.NewToolSet(tools.Deps{Resolver: resA, Browser: bridge}, "RUN", minterA.Mint("RUN")),
		b:    tools.NewToolSet(tools.Deps{Resolver: resB, Browser: bridge}, "RUN", minterB.Mint("RUN")),
	}
}

func call(t *testing.T, ts *tools.ToolSet, name, args string) (string, error) {
	t.Helper()
	return ts.Call(context.Background(), name, json.RawMessage(args))
}

func TestBrowserTools_AbsentWithoutABridge(t *testing.T) {
	res, minter := resolverOnChat(t, "chat-a")
	ts := tools.NewToolSet(tools.Deps{Resolver: res}, "RUN", minter.Mint("RUN"))
	for _, tool := range ts.Tools() {
		require.NotContains(t, tool.Name, "browser_")
	}
}

func TestBrowserTools_AreAdvertised(t *testing.T) {
	w := newBrowserWorld(t)
	names := map[string]bool{}
	for _, tool := range w.a.Tools() {
		names[tool.Name] = true
		require.True(t, json.Valid(tool.InputSchema), tool.Name)
	}
	for _, want := range []string{
		"browser_open", "browser_tabs", "browser_navigate", "browser_back", "browser_forward",
		"browser_reload", "browser_snapshot", "browser_click", "browser_type", "browser_press_key",
		"browser_scroll", "browser_wait", "browser_screenshot", "browser_eval", "browser_close",
	} {
		require.True(t, names[want], want)
	}
}

func TestBrowserOpen_RunsOnTheCallersOwnChat(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reply = `{"tab":"t1"}`

	out, err := call(t, w.a, "browser_open", `{"url":"https://example.com"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"tab":"t1"}`, out)

	got := w.peer.ran()
	require.Len(t, got, 1)
	require.Equal(t, "open", got[0]["op"])
	require.Equal(t, "chat-a", got[0]["chatId"])
	require.JSONEq(t, `{"url":"https://example.com"}`, mustJSON(got[0]["args"]))
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func TestBrowserTools_AgentSeesOnlyItsOwnChatsTabs(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"mine", "chat-a"}, [2]string{"theirs", "chat-b"})

	out, err := call(t, w.a, "browser_tabs", `{}`)
	require.NoError(t, err)
	require.Contains(t, out, `"mine"`)
	require.NotContains(t, out, "theirs")

	out, err = call(t, w.b, "browser_tabs", `{}`)
	require.NoError(t, err)
	require.Contains(t, out, `"theirs"`)
	require.NotContains(t, out, "mine")
}

func TestBrowserTools_AgentCannotActOnAnotherAgentsTab(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"mine", "chat-a"}, [2]string{"theirs", "chat-b"})

	for _, tc := range []struct{ tool, args string }{
		{"browser_navigate", `{"tab":"theirs","url":"https://evil.test"}`},
		{"browser_eval", `{"tab":"theirs","js":"return document.cookie"}`},
		{"browser_snapshot", `{"tab":"theirs"}`},
		{"browser_screenshot", `{"tab":"theirs"}`},
		{"browser_close", `{"tab":"theirs"}`},
		{"browser_click", `{"tab":"theirs","ref":"e1"}`},
		{"browser_eval", `{"tab":"guessed","js":"return 1"}`},
	} {
		_, err := call(t, w.a, tc.tool, tc.args)
		require.ErrorIs(t, err, browserbridge.ErrUnknownTab, tc.tool+" "+tc.args)
	}
	require.Empty(t, w.peer.ran(), "no forbidden call may reach the desktop")
}

func TestBrowserTools_ForgedScopeInArgumentsIsRejected(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"theirs", "chat-b"})

	_, err := call(t, w.a, "browser_open", `{"url":"https://a.test","chatId":"chat-b"}`)
	require.ErrorContains(t, err, "unknown argument")
	require.Empty(t, w.peer.ran())
}

func TestBrowserTools_DefaultTabIsTheOnlyOne(t *testing.T) {
	w := newBrowserWorld(t)

	_, err := call(t, w.a, "browser_snapshot", `{}`)
	require.ErrorContains(t, err, "browser_open")

	w.peer.reportTabs([2]string{"t1", "chat-a"})
	out, err := call(t, w.a, "browser_snapshot", `{}`)
	require.NoError(t, err)
	require.Equal(t, "ok", out)
	require.Equal(t, "t1", w.peer.ran()[0]["tab"])

	w.peer.reportTabs([2]string{"t1", "chat-a"}, [2]string{"t2", "chat-a"})
	_, err = call(t, w.a, "browser_snapshot", `{}`)
	require.ErrorContains(t, err, "pass tab")
}

func TestBrowserTools_RequiredArgumentsAreChecked(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"t1", "chat-a"})
	_, err := call(t, w.a, "browser_click", `{}`)
	require.ErrorContains(t, err, "ref is required")
	require.Empty(t, w.peer.ran())
}

func TestBrowserWait_TimeoutIsBounded(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"t1", "chat-a"})

	_, err := call(t, w.a, "browser_wait", `{"condition":"load","timeoutMs":9999999}`)
	require.NoError(t, err)
	_, err = call(t, w.a, "browser_wait", `{"condition":"load"}`)
	require.NoError(t, err)

	ran := w.peer.ran()
	require.JSONEq(t, `{"condition":"load","timeoutMs":30000}`, mustJSON(ran[0]["args"]))
	require.JSONEq(t, `{"condition":"load","timeoutMs":10000}`, mustJSON(ran[1]["args"]))
}

func TestBrowserScreenshot_ReturnsAnImage(t *testing.T) {
	w := newBrowserWorld(t)
	w.peer.reportTabs([2]string{"t1", "chat-a"})
	w.peer.reply = `{"png":"iVBORw0KGgo=","width":800,"height":600}`

	text, images, err := w.a.CallWithImages(context.Background(), "browser_screenshot", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "Screenshot 800x600", text)
	require.Len(t, images, 1)
	require.Equal(t, "image/png", images[0].MIMEType)
	require.Equal(t, "iVBORw0KGgo=", images[0].Data)

	w.peer.reply = `{}`
	_, _, err = w.a.CallWithImages(context.Background(), "browser_screenshot", json.RawMessage(`{}`))
	require.ErrorContains(t, err, "no image")
}

func TestBrowserTools_NoDesktopIsAClearError(t *testing.T) {
	bridge := browserbridge.New()
	res, minter := resolverOnChat(t, "chat-a")
	ts := tools.NewToolSet(tools.Deps{Resolver: res, Browser: bridge}, "RUN", minter.Mint("RUN"))
	_, err := call(t, ts, "browser_open", `{}`)
	require.ErrorIs(t, err, browserbridge.ErrNoDesktop)
}

// resolverOnChat is a resolver whose runner is currently on chatID, in one workspace
// shared by every chat: two agents there must still not share a browser.
func resolverOnChat(t *testing.T, chatID string) (*tools.Resolver, *tools.TokenMinter) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	return tools.NewResolver(
		m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: chatID, WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: chatID, WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()},
	), m
}
