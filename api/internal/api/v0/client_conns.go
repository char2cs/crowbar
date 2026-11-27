package v0

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/gin-gonic/gin"
)

var errClientRevoked = errors.New("v0: client was revoked during the upgrade")

type clientConns struct {
	mu    sync.Mutex
	conns map[string]map[*trackedConn]struct{}
}

func newClientConns() *clientConns {
	return &clientConns{conns: map[string]map[*trackedConn]struct{}{}}
}

func (r *clientConns) Disconnect(
	clientID string,
) {
	r.mu.Lock()
	held := r.conns[clientID]
	delete(r.conns, clientID)
	r.mu.Unlock()
	for conn := range held {
		if err := conn.Close(); err != nil {
			slog.Debug("v0: disconnect a revoked client's stream", "client", clientID, "err", err)
		}
	}
}

func (r *clientConns) track(
	clientID string,
	conn net.Conn,
) *trackedConn {
	tracked := &trackedConn{Conn: conn, owner: r, clientID: clientID}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.conns[clientID] == nil {
		r.conns[clientID] = map[*trackedConn]struct{}{}
	}
	r.conns[clientID][tracked] = struct{}{}
	return tracked
}

func (r *clientConns) untrack(
	tracked *trackedConn,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	held := r.conns[tracked.clientID]
	delete(held, tracked)
	if len(held) == 0 {
		delete(r.conns, tracked.clientID)
	}
}

type trackedConn struct {
	net.Conn
	owner     *clientConns
	clientID  string
	closeOnce sync.Once
}

func (c *trackedConn) Close() error {
	c.closeOnce.Do(func() { c.owner.untrack(c) })
	if err := c.Conn.Close(); err != nil {
		return fmt.Errorf("v0: close client conn: %w", err)
	}
	return nil
}

type revocableWriter struct {
	gin.ResponseWriter
	ctx       context.Context
	clients   clientAuthStore
	conns     *clientConns
	clientID  string
	tokenHash string
}

func (w *revocableWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := w.ResponseWriter.Hijack()
	if err != nil {
		return nil, nil, fmt.Errorf("v0: hijack: %w", err)
	}
	tracked := w.conns.track(w.clientID, conn)
	client, err := pairedClient(w.ctx, w.clients, w.tokenHash)
	if err == nil && client != nil {
		return tracked, rw, nil
	}
	return nil, nil, errors.Join(errClientRevoked, err, tracked.Close())
}
