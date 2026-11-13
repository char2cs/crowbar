package apidriver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

const waitFor = 5 * time.Second

// link is one fake connection: a scripted peer answering every request, and the
// methods it saw (with the session id a call named, if any).
type link struct {
	in    chan []byte
	out   chan []byte
	done  chan struct{}
	once  sync.Once
	calls chan string
}

func newLink() *link {
	l := &link{
		in: make(chan []byte, 8), out: make(chan []byte, 8),
		done: make(chan struct{}), calls: make(chan string, 16),
	}
	go l.serve()
	return l
}

func (l *link) Send(b []byte) error {
	select {
	case l.out <- b:
		return nil
	case <-l.done:
		return io.ErrClosedPipe
	}
}

func (l *link) Recv() ([]byte, error) {
	select {
	case b, ok := <-l.in:
		if !ok {
			return nil, io.EOF
		}
		return b, nil
	case <-l.done:
		return nil, io.EOF
	}
}

func (l *link) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *link) serve() {
	results := map[string]string{"open": `{"id":"s1"}`}
	for {
		var b []byte
		select {
		case b = <-l.out:
		case <-l.done:
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ID string `json:"id"`
			} `json:"params"`
		}
		if json.Unmarshal(b, &req) != nil {
			return
		}
		l.calls <- req.Method + ":" + req.Params.ID
		if len(req.ID) == 0 {
			continue
		}
		result := results[req.Method]
		if result == "" {
			result = `{}`
		}
		l.in <- []byte(`{"id":` + string(req.ID) + `,"result":` + result + `}`)
	}
}

func (l *link) drop() { close(l.in) }

func (l *link) notify(method string) { l.in <- []byte(`{"method":"` + method + `","params":{}}`) }

// next fails the test unless the peer's next call is want.
func (l *link) next(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-l.calls:
		require.Equal(t, want, got)
	case <-time.After(waitFor):
		t.Fatalf("timed out waiting for %s", want)
	}
}

func reconnectDescriptor() *spec.Descriptor {
	return &spec.Descriptor{
		ID: "fake",
		Runtime: spec.RuntimeSpec{API: spec.APISpec{
			Handshake: spec.HandshakeSpec{Call: "hello", Notify: "ready"},
		}},
		Events: map[string]spec.EventSpec{
			"prompt": {
				Fresh:  []spec.CallStep{{Call: "open", Capture: map[string]string{"session_id": "id"}}},
				Resume: []spec.CallStep{{Call: "enter", Send: map[string]any{"id": "{session_id}"}}},
				Action: []spec.CallStep{{Call: "say"}},
			},
			"turn_stop":    {API: &spec.ChannelBlock{In: spec.WireRef{"done"}}},
			"notification": {API: &spec.ChannelBlock{In: spec.WireRef{"note"}}},
			"turn_failed": {API: &spec.ChannelBlock{
				In:  spec.WireRef{"failed"},
				Map: spec.FieldMap{"reason": mapping.Paths("why"), "session_id": mapping.Paths("sid")},
			}},
		},
	}
}

// dialer hands out links in order; a nil entry is a failed dial.
func dialer(links ...*link) func(context.Context) (jsonrpc.Transport, error) {
	var mu sync.Mutex
	return func(context.Context) (jsonrpc.Transport, error) {
		mu.Lock()
		defer mu.Unlock()
		if len(links) == 0 || links[0] == nil {
			if len(links) > 0 {
				links = links[1:]
			}
			return nil, errors.New("dial refused")
		}
		l := links[0]
		links = links[1:]
		return l, nil
	}
}

func instantSleep(slept chan time.Duration) func(context.Context, time.Duration) error {
	return func(_ context.Context, d time.Duration) error {
		slept <- d
		return nil
	}
}

func startDial(t *testing.T, dial func(context.Context) (jsonrpc.Transport, error), rc apidriver.Reconnect) *apidriver.Driver {
	t.Helper()
	drv, err := apidriver.StartDial(t.Context(), reconnectDescriptor(), dial, rc, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = drv.Close() })
	return drv
}

func nextEvent(t *testing.T, drv *apidriver.Driver) apidriver.Event {
	t.Helper()
	select {
	case ev, ok := <-drv.Events():
		require.True(t, ok, "events closed")
		return ev
	case <-time.After(waitFor):
		t.Fatal("timed out waiting for an event")
		return apidriver.Event{}
	}
}

func handshakeCalls(t *testing.T, l *link) {
	t.Helper()
	l.next(t, "hello:")
	l.next(t, "ready:")
}

func TestReconnect_DropMidTurnFailsTheTurnThenResumesWithoutReplay(t *testing.T) {
	first, second := newLink(), newLink()
	slept := make(chan time.Duration, 4)
	drv := startDial(t, dialer(first, second), apidriver.Reconnect{Max: 3, Backoff: time.Second, Sleep: instantSleep(slept)})
	handshakeCalls(t, first)
	_, err := drv.Dispatch(t.Context(), "prompt", map[string]string{"text": "hi"})
	require.NoError(t, err)
	first.next(t, "open:")
	first.next(t, "say:")

	first.drop()

	ev := nextEvent(t, drv)
	require.Equal(t, "turn_failed", ev.Canonical)
	var raw map[string]string
	require.NoError(t, json.Unmarshal(ev.Raw, &raw))
	require.Equal(t, "s1", raw["sid"])
	require.NotEmpty(t, raw["why"])
	handshakeCalls(t, second)
	second.next(t, "enter:s1")
	second.notify("note")
	require.Equal(t, "notification", nextEvent(t, drv).Canonical, "the stream continues on the new connection")
	select {
	case got := <-second.calls:
		t.Fatalf("the prompt was replayed: %s", got)
	default:
	}
}

func TestReconnect_IdleDropResumesTheSameSessionWithNoFailureEvent(t *testing.T) {
	first, second := newLink(), newLink()
	slept := make(chan time.Duration, 4)
	drv := startDial(t, dialer(first, second), apidriver.Reconnect{Max: 3, Backoff: time.Second, Sleep: instantSleep(slept)})
	handshakeCalls(t, first)
	_, err := drv.EstablishSession(t.Context(), "prompt", nil)
	require.NoError(t, err)
	first.next(t, "open:")

	first.notify("done") // the turn ended before the drop
	require.Equal(t, "turn_stop", nextEvent(t, drv).Canonical)
	first.drop()

	handshakeCalls(t, second)
	second.next(t, "enter:s1")
	second.notify("note")
	require.Equal(t, "notification", nextEvent(t, drv).Canonical)
	require.Equal(t, time.Second, <-slept)
}

func TestReconnect_ExhaustedRetriesCloseEvents(t *testing.T) {
	first := newLink()
	slept := make(chan time.Duration, 4)
	drv := startDial(t, dialer(first, nil, nil), apidriver.Reconnect{Max: 2, Backoff: time.Second, Sleep: instantSleep(slept)})
	handshakeCalls(t, first)

	first.drop()

	select {
	case _, ok := <-drv.Events():
		require.False(t, ok)
	case <-time.After(waitFor):
		t.Fatal("events never closed")
	}
	require.Len(t, slept, 2)
}

func TestReconnect_CloseDuringRedialEndsTheDriver(t *testing.T) {
	first := newLink()
	redialing := make(chan struct{})
	var dials int
	dial := func(ctx context.Context) (jsonrpc.Transport, error) {
		dials++
		if dials == 1 {
			return first, nil
		}
		close(redialing)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	drv := startDial(t, dial, apidriver.Reconnect{Max: 3, Backoff: time.Second, Sleep: instantSleep(make(chan time.Duration, 4))})
	handshakeCalls(t, first)
	first.drop()
	<-redialing

	require.NoError(t, drv.Close())

	select {
	case _, ok := <-drv.Events():
		require.False(t, ok)
	case <-time.After(waitFor):
		t.Fatal("events never closed")
	}
}

func TestStart_StdioDropStillClosesEvents(t *testing.T) {
	stdio := fakeCodexServer(t, func(peer *testutil.LinePeer) { _ = peer.Close() })
	drv, err := apidriver.Start(t.Context(), loadCodexAPIDescriptor(t), stdio, nil)
	require.NoError(t, err)
	defer func() { _ = drv.Close() }()

	select {
	case _, ok := <-drv.Events():
		require.False(t, ok)
	case <-time.After(waitFor):
		t.Fatal("events never closed")
	}
}
