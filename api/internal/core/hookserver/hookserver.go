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

var hookRoute = regexp.MustCompile(
	`^/v0/projects/[^/]+/(repos/[^/]+|home)/chats/runners/[^/]+/hooks/[^/]+/[^/]+$`,
)

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

type Server struct {
	listener net.Listener
	server   *http.Server
}

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

func (s *Server) URL() string {
	return "http://" + s.listener.Addr().String()
}

func (s *Server) Serve() error {
	return s.server.Serve(s.listener)
}

func (s *Server) Shutdown(
	ctx context.Context,
) error {
	return s.server.Shutdown(ctx)
}

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

func Read(
	home string,
) (string, error) {
	b, err := os.ReadFile(endpointFile(home))
	if err != nil {
		return "", fmt.Errorf("hookserver: read endpoint: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}
