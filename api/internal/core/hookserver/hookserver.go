// Package hookserver is the daemon's loopback listener for the vendor CLIs that
// post their hooks themselves (native HTTP hook delivery) instead of running a
// relay process per event.
//
// It serves exactly one route — the runner-keyed hook route — and nothing else of
// the daemon's API, which is unauthenticated and so must never be reachable from a
// TCP port any local process can dial. The hook route authenticates itself with the
// per-runner token.
package hookserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// hookRoute is .../chats/runners/:segid/hooks/:provider/:event under a repo or
// under the project home.
var hookRoute = regexp.MustCompile(
	`^/v0/projects/[^/]+/(repos/[^/]+|home)/chats/runners/[^/]+/hooks/[^/]+/[^/]+$`,
)

// Gate lets only a POST to the hook route through to next.
func Gate(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !hookRoute.MatchString(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Server is the loopback listener.
type Server struct {
	listener net.Listener
	server   *http.Server
}

// Listen binds an ephemeral port on 127.0.0.1 and gates next to the hook route.
func Listen(
	ctx context.Context,
	next http.Handler,
) (*Server, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("hookserver: listen: %w", err)
	}
	return &Server{
		listener: ln,
		server:   &http.Server{Handler: Gate(next), ReadHeaderTimeout: 30 * time.Second},
	}, nil
}

// URL is the server's base URL, e.g. http://127.0.0.1:49152.
func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String()
}

// Serve serves until Shutdown or Close, which make it return http.ErrServerClosed.
func (s *Server) Serve() error {
	return s.server.Serve(s.listener)
}

// Shutdown stops accepting and waits for in-flight requests, up to ctx.
func (s *Server) Shutdown(
	ctx context.Context,
) error {
	return s.server.Shutdown(ctx)
}

// Close closes the listener and every connection. It closes the listener itself
// too: until Serve runs, the http.Server does not track it.
func (s *Server) Close() error {
	err := s.server.Close()
	if lerr := s.listener.Close(); lerr != nil && !errors.Is(lerr, net.ErrClosed) {
		err = errors.Join(err, fmt.Errorf("hookserver: close listener: %w", lerr))
	}
	return err
}

func endpointFile(
	home string,
) string {
	return filepath.Join(home, "run", "hooks.url")
}

// Publish records the server's URL under home, owner-only, for the spawner to read
// when it renders a CLI's hook configuration. It is rewritten at every daemon boot:
// a runner is a child of the daemon, so none outlives the URL it was handed.
func Publish(
	home, url string,
) error {
	path := endpointFile(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("hookserver: publish: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(url+"\n"), 0o600); err != nil {
		return fmt.Errorf("hookserver: publish: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("hookserver: publish: %w", err)
	}
	return nil
}

// Read returns the URL the running daemon published under home.
func Read(
	home string,
) (string, error) {
	b, err := os.ReadFile(endpointFile(home))
	if err != nil {
		return "", fmt.Errorf("hookserver: read endpoint: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
