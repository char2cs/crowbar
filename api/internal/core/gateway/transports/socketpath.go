package transports

import "strings"

// SocketPath resolves the daemon's unix socket path from a host string
// ("unix://" or "unix:///abs/path"). With no explicit path it derives the
// same location the daemon binds: $TMPDIR/crowbar-<fnv1a64(CROWBAR_HOME)>.sock
// when CROWBAR_HOME is set, else ~/.crowbar/crowbar.sock. Shared by the daemon
// (NewSocket) and unix-socket clients (e.g. `crowbar hook`).
func SocketPath(host string) (string, error) {
	return socketPath(strings.TrimPrefix(host, "unix://"))
}
