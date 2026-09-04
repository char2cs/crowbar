package main

import (
	"context"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
)

const tcpScheme = "tcp://"

// transport is the three calls this tool needs from a daemon connection. All
// three report the HTTP status separately from err, and all return err == nil
// for a non-2xx — decodeEnvelope is what turns a daemon refusal into a
// failure. PatchJSON is used by exactly one call: renaming a freshly forked
// chat's server-generated branch back to the seed's own fixed name (see
// ensureFeatureChat).
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

// newTransport dials whichever wire the dev daemon is on. `make dev-desktop`
// runs its sidecar on a unix socket whose path is an FNV hash of CROWBAR_HOME —
// ipc.NewClient derives that, which is why this tool is a Go program and not a
// shell script — while `make dev-api` serves plain TCP on 127.0.0.1:3737.
func newTransport(
	host string,
) (transport, error) {
	if strings.HasPrefix(host, tcpScheme) {
		return newTCPTransport(host), nil
	}
	return ipc.NewClient(host)
}
