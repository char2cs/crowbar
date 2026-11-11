package internal

import (
	"fmt"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/gateway"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
)

// serverIdentity loads the TLS identity, and its fingerprint, for a
// network-exposed bind only: a unix-socket or loopback daemon serves no TLS,
// so a broken identity directory must not keep it from booting.
func serverIdentity(
	host string,
	h home.Home,
) (*tlsid.Identity, string, error) {
	if !gateway.Exposed(host) {
		return nil, "", nil
	}
	identity, err := tlsid.Load(filepath.Join(h.Root(), "tls"))
	if err != nil {
		return nil, "", fmt.Errorf("internal: tls identity: %w", err)
	}
	return identity, identity.Fingerprint, nil
}
