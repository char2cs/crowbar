package main

import (
	"context"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

const tcpScheme = "tcp://"

// transport is the three calls this tool needs from a daemon connection. Each
// reports the HTTP status separately and returns err == nil for a non-2xx;
// decodeEnvelope is what turns a daemon refusal into a failure.
type transport interface {
	Get(
		ctx context.Context,
		path string,
	) (int, []byte, error)
	PostJSON(
		ctx context.Context,
		path string,
		body any,
	) (int, []byte, error)
	PatchJSON(
		ctx context.Context,
		path string,
		body any,
	) (int, []byte, error)
}

// newTransport dials whichever wire the dev daemon is on: `make dev-desktop` uses
// a unix socket whose path ipc.NewClient derives from CROWBAR_HOME, while
// `make dev-api` serves plain TCP on 127.0.0.1:3737.
func newTransport(
	host string,
) (transport, error) {
	if strings.HasPrefix(host, tcpScheme) {
		return newTCPTransport(host), nil
	}
	return ipc.NewClient(host)
}
