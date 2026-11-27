package tools

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

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

const relayDomain = "relay:"

func (m *TokenMinter) MintRelay(relayID string) string {
	return base64.RawURLEncoding.EncodeToString(m.sign(relayDomain + relayID))
}

func (m *TokenMinter) VerifyRelay(relayID, token string) bool {
	if relayID == "" || token == "" {
		return false
	}
	got, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}
	return hmac.Equal(got, m.sign(relayDomain+relayID))
}

func (m TokenMinter) String() string {
	return "agenttools.TokenMinter{secret:REDACTED}"
}

func (m TokenMinter) GoString() string {
	return m.String()
}

func (m *TokenMinter) Verify(runnerID, token string) bool {
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
