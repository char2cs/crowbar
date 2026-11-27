package apidriver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/mapping"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/httpdriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func StartHTTP(ctx context.Context, d *spec.Descriptor, c *httpdriver.Client, origin SessionOrigin) (*Driver, error) {
	return newDriver(ctx, d, newHTTPWire(c), nil, Reconnect{}, origin)
}

type httpWire struct {
	c      *httpdriver.Client
	frames chan jsonrpc.Frame
	ctx    context.Context
	cancel context.CancelFunc
}

func newHTTPWire(c *httpdriver.Client) *httpWire {
	w := &httpWire{c: c, frames: make(chan jsonrpc.Frame, 64)}
	w.ctx, w.cancel = context.WithCancel(context.Background())
	go w.forward()
	return w
}

func (w *httpWire) forward() {
	defer close(w.frames)
	for f := range w.c.Frames() {
		if f.Err != nil {
			slog.WarnContext(w.ctx, "apidriver: stream frame dropped", "err", f.Err)
			continue
		}
		select {
		case w.frames <- jsonrpc.Frame{Method: f.Method, Params: f.Params}:
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *httpWire) Call(ctx context.Context, name string, params any) (json.RawMessage, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("apidriver: encode %s: %w", name, err)
	}
	return w.c.Call(ctx, name, raw)
}

func (*httpWire) Notify(string, any) error {
	return errors.New("apidriver: http delivery has no notifications")
}

func (w *httpWire) Reply(id, result json.RawMessage) error {
	var ask struct{ Request, ID string }
	if err := json.Unmarshal(id, &ask); err != nil {
		return fmt.Errorf("apidriver: ask id: %w", err)
	}
	return w.c.Reply(w.ctx, ask.Request, ask.ID, result)
}

func (*httpWire) ReplyError(json.RawMessage, int, string) error {
	return errors.New("apidriver: http delivery has no server requests to refuse")
}

func (w *httpWire) Frames() <-chan jsonrpc.Frame { return w.frames }

func (*httpWire) Overflowed() bool { return false }

func (w *httpWire) Close() error {
	w.cancel()
	return w.c.Close()
}

func (drv *Driver) httpAskID(canonical string, params map[string]any) json.RawMessage {
	ev := drv.d.Events[canonical]
	if drv.d.Runtime.API.Delivery != spec.APIDeliveryHTTP || ev.ReplyRequest == "" {
		return nil
	}
	var id string
	if ev.API != nil {
		id, _ = mapping.Scalar(params, ev.API.Map["ask_id"])
	}
	raw, err := json.Marshal(map[string]string{"request": ev.ReplyRequest, "id": id})
	if err != nil {
		return nil
	}
	return raw
}
