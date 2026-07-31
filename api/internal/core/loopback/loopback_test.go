package loopback_test

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/loopback"
)

func TestIssue_MintsAFreshTokenPerCall(t *testing.T) {
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321}

	first, err := loopback.Issue(addr)
	require.NoError(t, err)
	second, err := loopback.Issue(addr)
	require.NoError(t, err)

	assert.NotEqual(t, first.Token, second.Token, "a token must never be reused across boots")
	assert.Len(t, first.Token, 43, "256 bits of entropy is 43 unpadded base64url characters")
	assert.NotRegexp(t, `[^A-Za-z0-9_-]`, first.Token, "the token must be URL-safe so it survives a query parameter")

	assert.Equal(t, loopback.CredentialsVersion, first.Version)
	assert.Equal(t, "http", first.Scheme)
	assert.Equal(t, "127.0.0.1:4321", first.Address)
	assert.Equal(t, 4321, first.Port)
	assert.Equal(t, "http://127.0.0.1:4321", first.URL)
	assert.Equal(t, os.Getpid(), first.PID)
}

func TestIssue_NonTCPAddress_ReturnsError(t *testing.T) {
	_, err := loopback.Issue(&net.UnixAddr{Name: "/tmp/x.sock", Net: "unix"})
	assert.Error(t, err)
}

func TestCredentials_Publish_WritesOwnerOnlyJSON(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	creds, err := loopback.Issue(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321})
	require.NoError(t, err)

	path, err := creds.Publish(stateDir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(stateDir, loopback.FileName), path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var decoded loopback.Credentials
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, *creds, decoded)
}

// TestCredentials_Publish_OverwritesWithoutWideningPermissions covers the republish
// path: a second boot must not inherit a loosened mode from the file the previous
// one left behind.
func TestCredentials_Publish_OverwritesWithoutWideningPermissions(t *testing.T) {
	stateDir := t.TempDir()
	creds, err := loopback.Issue(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321})
	require.NoError(t, err)

	stale := filepath.Join(stateDir, loopback.FileName)
	require.NoError(t, os.WriteFile(stale, []byte("{}"), 0o644))

	path, err := creds.Publish(stateDir)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestCredentials_Publish_UnwritableStateDir_ReturnsError(t *testing.T) {
	creds, err := loopback.Issue(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321})
	require.NoError(t, err)

	_, err = creds.Publish("/dev/null/not-a-directory")
	assert.Error(t, err)
}

// TestCredentials_Redaction proves the token cannot escape through a formatting
// verb: %v, %s and a structured log value all render the redacted form.
func TestCredentials_Redaction(t *testing.T) {
	creds, err := loopback.Issue(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321})
	require.NoError(t, err)

	rendered := creds.String()
	assert.NotContains(t, rendered, creds.Token)
	assert.Contains(t, rendered, "REDACTED")
	assert.NotContains(t, creds.LogValue().String(), creds.Token)
}

func TestRevoke_RemovesThePublication(t *testing.T) {
	stateDir := t.TempDir()
	creds, err := loopback.Issue(&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4321})
	require.NoError(t, err)
	path, err := creds.Publish(stateDir)
	require.NoError(t, err)

	require.NoError(t, loopback.Revoke(path))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))

	assert.NoError(t, loopback.Revoke(path), "revoking an already-gone file is not an error")
	assert.NoError(t, loopback.Revoke(""), "revoking nothing is not an error")
}

// TestAddress_ResolvesFlagsAndEnv pins the configuration surface: the listener is
// OFF unless something asks for it, and an explicit address implies the ask.
func TestAddress_ResolvesFlagsAndEnv(t *testing.T) {
	cases := []struct {
		name        string
		flagEnabled bool
		flagAddr    string
		envEnable   string
		envAddr     string
		want        string
	}{
		{name: "off by default", want: ""},
		{name: "flag enables the ephemeral default", flagEnabled: true, want: loopback.DefaultAddress},
		{name: "flag address implies enabled", flagAddr: "127.0.0.1:9999", want: "127.0.0.1:9999"},
		{name: "env enables the ephemeral default", envEnable: "1", want: loopback.DefaultAddress},
		{name: "env true", envEnable: "TRUE", want: loopback.DefaultAddress},
		{name: "env yes", envEnable: "yes", want: loopback.DefaultAddress},
		{name: "env on", envEnable: "on", want: loopback.DefaultAddress},
		{name: "env false stays off", envEnable: "0", want: ""},
		{name: "env nonsense stays off", envEnable: "maybe", want: ""},
		{name: "env address implies enabled", envAddr: "127.0.0.1:8888", want: "127.0.0.1:8888"},
		{name: "env address is trimmed", envAddr: "  127.0.0.1:8888 ", want: "127.0.0.1:8888"},
		{
			name:     "flag address beats env address",
			flagAddr: "127.0.0.1:7777",
			envAddr:  "127.0.0.1:8888",
			want:     "127.0.0.1:7777",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(loopback.EnvEnable, tc.envEnable)
			t.Setenv(loopback.EnvAddress, tc.envAddr)
			assert.Equal(t, tc.want, loopback.Address(tc.flagEnabled, tc.flagAddr))
		})
	}
}
