package hub_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/hub"
)

func TestHubBroadcast(t *testing.T) {
	h := hub.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go h.Run(ctx)

	client := h.Register()
	defer h.Unregister(client)

	go h.Broadcast(hub.Event{Type: "ping", Data: "hello"})

	select {
	case evt := <-client:
		if evt.Type != "ping" {
			t.Fatalf("expected type ping, got %s", evt.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}
