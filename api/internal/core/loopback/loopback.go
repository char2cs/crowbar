// Package loopback issues, publishes and verifies the bearer credential that
// guards the daemon's auxiliary loopback TCP listener.
//
// The daemon's production transport is a unix socket, whose access control is
// the filesystem: the socket is chmod 0600, so only the owning user's processes
// can connect. A TCP listener has NO equivalent — every process on the box, of
// every user, plus any page running in a browser on it, can open a connection to
// 127.0.0.1. The API behind that connection spawns shells, reads and writes
// arbitrary files and drives git, so the TCP listener must not be reachable
// without a credential. That credential is this package.
//
// The token is minted fresh on every boot from crypto/rand, never persisted
// across restarts, and published alongside the bound port in a 0600 file inside
// the crowbar state directory. The unix listener is deliberately left alone: it
// gains no new credential requirement, so existing clients are unaffected.
package loopback

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"strings"
)

// FileName is the base name of the published credentials file inside the crowbar
// state directory ($CROWBAR_HOME/state/loopback.json).
const FileName = "loopback.json"

// CredentialsVersion is the schema version stamped into the published file.
const CredentialsVersion = 1

// TokenBytes is the entropy behind an issued token: 256 bits, rendered as 43
// unpadded base64url characters.
const TokenBytes = 32

// HeaderName is the Crowbar-specific request header a client may use to present
// the token, as an alternative to "Authorization: Bearer <token>".
const HeaderName = "X-Crowbar-Token"

// QueryParam is the query-string parameter a client may use to present the
// token. It exists for the one caller that cannot set a request header: a
// browser WebSocket handshake (the WebSocket API has no header argument).
const QueryParam = "crowbar_token"

// DefaultAddress asks the OS for an ephemeral loopback port. A fixed default
// port would collide the moment two checkouts of this repo run at once, so the
// port is assigned and then published rather than assumed.
const DefaultAddress = "127.0.0.1:0"

// EnvEnable enables the auxiliary loopback TCP listener when set to a truthy
// value ("1", "true", "yes", "on"). It is the environment counterpart of the
// `crowbar serve --loopback-tcp` flag and is OFF when unset.
const EnvEnable = "CROWBAR_LOOPBACK_TCP"

// EnvAddress overrides the loopback bind address (default 127.0.0.1:0). Setting
// it also implies EnvEnable, so a caller that wants a specific port needs one
// variable rather than two.
const EnvAddress = "CROWBAR_LOOPBACK_TCP_ADDR"

// Issue mints a fresh per-boot token and describes the listener bound at addr.
// Call it with the listener's REAL address (listener.Addr(), not the requested
// bind string) so the published port is the one the OS actually assigned.
func Issue(
	addr net.Addr,
) (*Credentials, error) {
	raw := make([]byte, TokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("loopback: mint token: %w", err)
	}
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("loopback: expected a TCP address, got %T", addr)
	}
	return &Credentials{
		Version: CredentialsVersion,
		Scheme:  "http",
		Address: tcpAddr.String(),
		Port:    tcpAddr.Port,
		URL:     "http://" + tcpAddr.String(),
		Token:   base64.RawURLEncoding.EncodeToString(raw),
		PID:     os.Getpid(),
	}, nil
}

// Revoke removes a published credentials file. It runs on daemon shutdown so a
// dead daemon never leaves a token and a port behind for a client to trust; a
// file that is already gone is not an error.
func Revoke(
	path string,
) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loopback: revoke %s: %w", path, err)
	}
	return nil
}

// Address resolves the bind address for the auxiliary loopback TCP listener from
// the serve flags and their environment fallbacks, returning "" when the listener
// is disabled — which is the default, so nothing changes for an existing install
// that sets neither.
//
// flagAddr wins over EnvAddress; either one implies the listener is wanted, as
// does flagEnabled or a truthy EnvEnable. When it is wanted but no address was
// given, DefaultAddress applies.
func Address(
	flagEnabled bool,
	flagAddr string,
) string {
	addr := flagAddr
	if addr == "" {
		addr = strings.TrimSpace(os.Getenv(EnvAddress))
	}
	if addr != "" {
		return addr
	}
	if flagEnabled || truthy(os.Getenv(EnvEnable)) {
		return DefaultAddress
	}
	return ""
}

func truthy(
	value string,
) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
