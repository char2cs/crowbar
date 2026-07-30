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

// TokenMinter issues and verifies the per-runner token that binds an MCP call
// back into the daemon to the runner it claims to come from.
//
// What it is for. A runner id is not a credential — it is published on the chats
// API (dto.AgentChatDTO.LiveRunnerID) and it appears in argv. Requiring a token
// alongside it means a call must name a runner AND carry the thing minted for
// that runner, which
//
//   - stops a runner from acting as a sibling BY ACCIDENT — a copied command
//     line, a stale flag, a relay pointed at the wrong segment. These are the
//     realistic failures, and they fail closed instead of quietly succeeding
//     against someone else's workspace;
//   - keeps the relay unable to SELF-AUTHORIZE. `crowbar mcp` decides nothing:
//     it forwards bytes, and the daemon re-derives every caller from a
//     (runnerID, token) pair it minted itself. Authorization lives in exactly
//     one place because the transport has nothing to assert with.
//
// What it is NOT. It is not a containment boundary against an agent with a
// shell. The token rides in argv exactly as the id does, and on macOS
// `ps -Ao pid,args` shows full argv for every process of the same user — so an
// agent that can read a sibling's id can read its token by the same means. Nor
// is that the shortest path: the daemon's own HTTP surface has no
// authentication at all, so an agent with shell access can
// `curl --unix-socket` the full REST API and do strictly more than these tools
// permit. The MCP surface grants an adversarial agent no capability it did not
// already have; it grants a well-behaved one a scoped, attributable way to act.
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
