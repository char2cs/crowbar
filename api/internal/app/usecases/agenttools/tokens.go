// Package agenttools is the agent-facing capability surface: the tools an agent
// running inside a Crowbar chat may call, the authority model that decides what
// each caller can see, and the rendering of results back to a model.
//
// Everything here is reached through one seam — DispatchMCP — so authorization
// happens in exactly one place and the transport (an MCP relay process) never
// self-authorizes.
package agenttools

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// TokenMinter issues and verifies the per-runner token that authenticates an MCP
// call back into the daemon.
//
// The segment (runner) id alone is not an authenticator: the agent controls the
// process that holds it and can read its own argv, so an agent that learned a
// sibling's id could otherwise assume that sibling's scope. The token binds a
// caller to the runner it was minted for.
//
// The secret is per-DAEMON-BOOT and never persisted. That is sound because a
// runner's PTY is a child of the daemon: when the daemon dies every runner dies
// with it, so there is no live runner whose token could outlive the secret.
// Revocation is therefore automatic and no runner state has to be migrated.
type TokenMinter struct {
	secret []byte
}

func NewTokenMinter() (*TokenMinter, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("agenttools: mint token secret: %w", err)
	}
	return &TokenMinter{secret: secret}, nil
}

func (m *TokenMinter) Mint(runnerID string) string {
	return base64.RawURLEncoding.EncodeToString(m.sign(runnerID))
}

// String redacts the secret so no formatted print can leak it.
//
// The minter is held by the agent usecase, and any %v/%+v of a struct that
// reaches it — a debug log, a panic dump, an error built from a whole
// dependency graph — would otherwise render the raw HMAC key as bytes. GoString
// covers %#v for the same reason.
//
// The receiver is a VALUE, not a pointer, and that is load-bearing: a
// pointer-receiver String does not apply to a COPY of the minter, so printing a
// dereferenced or passed-by-value TokenMinter would fall back to the default
// struct formatter and print the raw key. A value receiver redacts both forms,
// because the method set of *TokenMinter includes it too.
func (m TokenMinter) String() string {
	return "agenttools.TokenMinter{secret:REDACTED}"
}

// GoString redacts the secret under %#v. See String.
func (m TokenMinter) GoString() string {
	return m.String()
}

// Verify is constant-time in the comparison so a caller cannot probe the token
// byte by byte.
func (m *TokenMinter) Verify(runnerID, token string) bool {
	// An empty runner id names no runner, so it must never authenticate — without
	// this guard Mint("") and Verify("", …) agree with each other and an unset
	// --segment flag would silently pass the check.
	if runnerID == "" || token == "" {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(got, m.sign(runnerID))
}

func (m *TokenMinter) sign(runnerID string) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write([]byte(runnerID))
	return mac.Sum(nil)
}
