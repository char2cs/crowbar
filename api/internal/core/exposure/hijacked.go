package exposure

import (
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"syscall"
)

// hijacked holds the connections handlers took over (terminal and watcher
// streams): http.Server.Close no longer owns them, so closing the listener
// has to end them itself.
type hijacked struct {
	mu    sync.Mutex
	conns []net.Conn
}

// watch records every connection srv hands to a handler.
func (h *hijacked) watch(srv *http.Server) {
	prev := srv.ConnState
	srv.ConnState = func(c net.Conn, s http.ConnState) {
		if prev != nil {
			prev(c, s)
		}
		if s == http.StateHijacked {
			h.add(c)
		}
	}
}

func (h *hijacked) add(c net.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	live := h.conns[:0]
	for _, held := range h.conns {
		if !ended(held) {
			live = append(live, held)
		}
	}
	h.conns = append(live, c)
}

func (h *hijacked) closeAll() {
	h.mu.Lock()
	held := h.conns
	h.conns = nil
	h.mu.Unlock()
	for _, c := range held {
		if err := c.Close(); err != nil && !ended(c) {
			slog.Debug("exposure: close hijacked stream", "component", "exposure", "err", err)
		}
	}
}

// ended reports a connection its owner already closed, without touching its
// deadlines or data; one that cannot be probed counts as live.
func ended(c net.Conn) bool {
	if t, ok := c.(*tls.Conn); ok {
		c = t.NetConn()
	}
	sc, ok := c.(syscall.Conn)
	if !ok {
		return false
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return true
	}
	return raw.Control(func(uintptr) {}) != nil
}
