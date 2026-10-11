package v0

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/gin-gonic/gin"
)

var errClientRevoked = errors.New("v0: client was revoked during the upgrade")

// clientConns holds every connection (a WebSocket) a paired client's request
// hijacked: the bearer guard only sees the upgrade, so revoking the client is
// what has to end the streams its token opened.
type clientConns struct {
	mu    sync.Mutex
	conns map[string]map[*trackedConn]struct{}
}

func newClientConns() *clientConns {
	return &clientConns{conns: map[string]map[*trackedConn]struct{}{}}
}

// Disconnect closes every connection clientID's requests hijacked.
func (r *clientConns) Disconnect(
	clientID string,
) {
	r.mu.Lock()
	held := r.conns[clientID]
	delete(r.conns, clientID)
	r.mu.Unlock()
	for conn := range held {
		_ = conn.Close()
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

// trackedConn leaves the registry when it closes, whoever closes it.
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

// revocableWriter registers a hijacked connection under its client, then
// re-reads the client: a revoke before the registration is caught here, one
// after finds it registered. ctx is the request's own; Hijack takes none.
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
	_ = tracked.Close()
	return nil, nil, errors.Join(errClientRevoked, err)
}
