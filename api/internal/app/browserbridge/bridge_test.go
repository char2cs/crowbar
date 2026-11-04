package browserbridge_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/browserbridge"
)

// fakePeer records every frame it is sent and answers through onSend.
type fakePeer struct {
	mu     sync.Mutex
	frames []map[string]any
	onSend func(f map[string]any)
}

func (p *fakePeer) Send(raw []byte) error {
	var f map[string]any
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	p.mu.Lock()
	p.frames = append(p.frames, f)
	p.mu.Unlock()
	if p.onSend != nil {
		p.onSend(f)
	}
	return nil
}

func (p *fakePeer) sent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.frames)
}

func tabsFrame(tabs ...string) []byte {
	items := []map[string]any{}
	for i := 0; i+1 < len(tabs); i += 2 {
		items = append(items, map[string]any{"id": tabs[i], "chatId": tabs[i+1], "url": "https://x.test/" + tabs[i]})
	}
	raw, _ := json.Marshal(map[string]any{"type": "tabs", "tabs": items})
	return raw
}

func respond(s *browserbridge.Session, ok bool, result, errText string) func(map[string]any) {
	return func(f map[string]any) {
		raw, _ := json.Marshal(map[string]any{
			"type": "response", "id": f["id"], "ok": ok,
			"result": json.RawMessage(result), "error": errText,
		})
		s.Receive(raw)
	}
}

func attach(b *browserbridge.Bridge) (*browserbridge.Session, *fakePeer) {
	p := &fakePeer{}
	return b.Attach(p), p
}

func TestDo_NoDesktopIsAClearError(t *testing.T) {
	_, err := browserbridge.New().Do(context.Background(), "chat1", "open", "", nil)
	require.ErrorIs(t, err, browserbridge.ErrNoDesktop)
}

func TestDo_RoundTripCarriesScopeFromTheCallNotTheArgs(t *testing.T) {
	b := browserbridge.New()
	s, p := attach(b)
	p.onSend = respond(s, true, `{"tab":"t1"}`, "")

	out, err := b.Do(context.Background(), "chat1", "open", "", json.RawMessage(`{"chatId":"forged"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"tab":"t1"}`, string(out))
	require.Equal(t, "chat1", p.frames[0]["chatId"])
	require.Equal(t, "open", p.frames[0]["op"])
}

func TestDo_PeerErrorSurfaces(t *testing.T) {
	b := browserbridge.New()
	s, p := attach(b)
	p.onSend = respond(s, false, `null`, "page is gone")
	_, err := b.Do(context.Background(), "chat1", "open", "", nil)
	require.ErrorContains(t, err, "page is gone")
}

func TestScope_AgentCannotListOrActOnAnotherChatsTab(t *testing.T) {
	b := browserbridge.New()
	s, p := attach(b)
	s.Receive(tabsFrame("mine", "chatA", "theirs", "chatB"))
	p.onSend = respond(s, true, `{}`, "")

	require.JSONEq(t, `[{"id":"mine","chatId":"chatA","url":"https://x.test/mine"}]`, string(b.Tabs("chatA")))
	require.JSONEq(t, `[]`, string(b.Tabs("chatC")))

	_, err := b.Do(context.Background(), "chatA", "eval", "mine", nil)
	require.NoError(t, err)

	before := p.sent()
	for _, id := range []string{"theirs", "guessed", "../theirs", "browser-theirs"} {
		_, err := b.Do(context.Background(), "chatA", "eval", id, nil)
		require.ErrorIs(t, err, browserbridge.ErrUnknownTab, id)
	}
	require.Equal(t, before, p.sent(), "a rejected call must never reach the desktop")
}

func TestScope_TabsInventoryIsReplacedWholesale(t *testing.T) {
	b := browserbridge.New()
	s, _ := attach(b)
	s.Receive(tabsFrame("a", "chat1", "b", "chat1"))
	s.Receive(tabsFrame("b", "chat1"))
	require.JSONEq(t, `[{"id":"b","chatId":"chat1","url":"https://x.test/b"}]`, string(b.Tabs("chat1")))
}

func TestScope_TabsRouteToTheWindowThatOwnsThem(t *testing.T) {
	b := browserbridge.New()
	s1, p1 := attach(b)
	s2, p2 := attach(b)
	s1.Receive(tabsFrame("one", "chat1"))
	s2.Receive(tabsFrame("two", "chat1"))
	p1.onSend = respond(s1, true, `{}`, "")
	p2.onSend = respond(s2, true, `{}`, "")

	_, err := b.Do(context.Background(), "chat1", "eval", "one", nil)
	require.NoError(t, err)
	require.Equal(t, 1, p1.sent())
	require.Equal(t, 0, p2.sent())
}

func TestDo_ClosingThePeerFailsItsPendingCallsAndForgetsItsTabs(t *testing.T) {
	b := browserbridge.New()
	s, p := attach(b)
	s.Receive(tabsFrame("a", "chat1"))
	p.onSend = func(map[string]any) { s.Close() }

	_, err := b.Do(context.Background(), "chat1", "eval", "a", nil)
	require.ErrorIs(t, err, browserbridge.ErrNoDesktop)
	require.JSONEq(t, `[]`, string(b.Tabs("chat1")))
}

func TestDo_CancelledContextEndsTheWait(t *testing.T) {
	b := browserbridge.New()
	_, p := attach(b)
	ctx, cancel := context.WithCancel(context.Background())
	p.onSend = func(map[string]any) { cancel() }

	_, err := b.Do(ctx, "chat1", "open", "", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestReceive_AnotherPeersAnswerDoesNotSettleTheCall(t *testing.T) {
	b := browserbridge.New()
	s1, p1 := attach(b)
	s2, _ := attach(b)
	s1.Receive(tabsFrame("one", "chat1"))
	ctx, cancel := context.WithCancel(context.Background())
	p1.onSend = func(f map[string]any) {
		respond(s2, true, `{"stolen":true}`, "")(f)
		cancel()
	}
	out, err := b.Do(ctx, "chat1", "eval", "one", nil)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, out)
}

func TestReceive_MalformedFramesAreDropped(t *testing.T) {
	b := browserbridge.New()
	s, _ := attach(b)
	s.Receive([]byte(`not json`))
	s.Receive([]byte(`{"type":"tabs","tabs":[{"id":"","chatId":"x"},{"id":"a"},7]}`))
	require.JSONEq(t, `[]`, string(b.Tabs("x")))
}
