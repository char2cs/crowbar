// Package tools is the agent-facing capability surface: the tools an agent in a Crowbar chat may call,
// the authority model deciding what each caller can see, and the rendering of results.
//
// It is reached through one seam (the chat usecase's DispatchMCP), so authorization happens in one place
// and the MCP relay transport never self-authorizes.
package tools

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// TokenMinter issues and verifies the per-runner token binding an MCP call to its runner. It stops
// accidental cross-runner calls and keeps the relay from self-authorizing; it is not a boundary against
// an agent with a shell. The secret lives per daemon boot, which every runner's PTY dies with.
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

// String redacts the secret so no %v/%+v of a struct reaching the minter leaks the HMAC key. The value
// receiver is load-bearing: it covers both a TokenMinter copy and a *TokenMinter.
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
	// Mint("") and Verify("", …) would agree, letting an unset --segment flag pass.
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
