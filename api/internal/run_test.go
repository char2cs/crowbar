package internal_test

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/tlsid"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

func TestClose_ReleasesTheHomeLock(t *testing.T) {
	for name, layout := range map[string]string{"serving": "", "halted": "9\n"} {
		t.Run(name, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			if layout != "" {
				testutil.WriteFile(t, h, "LAYOUT", layout)
			}
			c, err := internal.New(t.Context(), "unix://"+testutil.ShortSocket(t), nil, internal.WithHomeDir(h.Root()))
			require.NoError(t, err)

			c.Close()

			again, err := internal.New(t.Context(), "unix://"+testutil.ShortSocket(t), nil, internal.WithHomeDir(h.Root()))
			require.NoError(t, err)
			again.Close()
		})
	}
}

func TestRun_CancelledContext_Returns(t *testing.T) {
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = c.Run(ctx)
	assert.NoError(t, err)
}

func TestRun_ServesOnlyTheRunnerHookRouteOnALoopbackPort(t *testing.T) {
	home := t.TempDir()
	c, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	url, err := hookserver.Read(home)
	require.NoError(t, err, "the daemon publishes where hooks are served")
	require.True(t, strings.HasPrefix(url, "http://127.0.0.1:"), url)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	fetch := func(method, target string, payload io.Reader) string {
		req, err := http.NewRequestWithContext(ctx, method, target, payload)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(b)
	}
	assert.Contains(t, fetch(http.MethodPost, url+"/v0/projects/p/repos/r/chats/runners/seg/hooks/claude/tool_pre", strings.NewReader("{}")),
		"repository not found", "the real router, not the gate, answered")
	assert.Equal(t, "404 page not found\n", fetch(http.MethodGet, url+"/v0/projects", nil),
		"the gate refuses the rest of the API")
}

func TestRegression_DeviceAccess_OpenPairRevokeCloseReopen(t *testing.T) {
	h := testutil.FixtureHome(t)
	sock := testutil.ShortSocket(t)
	c, err := internal.New(context.Background(), "unix://"+sock, nil, internal.WithHomeDir(h.Root()))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	d := &device{t: t, local: testutil.UnixClient(t, sock), port: freePort(t)}
	d.tls = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	t.Cleanup(d.tls.CloseIdleConnections)

	var st accessBody
	d.local200(http.MethodGet, "/v0/access", "", &st)
	require.False(t, st.Enabled, "nothing listens until the owner turns it on")
	require.NotEmpty(t, st.Fingerprint, "the daemon's identity is known while it is closed")
	code, _ := d.do(d.local, "http://daemon", http.MethodPost, "/v0/pair/code", "", "")
	assert.Equal(t, http.StatusConflict, code, "no code while no device can reach the daemon")

	d.local200(http.MethodPut, "/v0/access", `{"enabled":true,"address":"0.0.0.0:`+d.port+`"}`, &st)
	require.True(t, st.Enabled)
	conn, err := d.dial()
	require.NoError(t, err)
	assert.Equal(t, st.Fingerprint, tlsid.FingerprintOf(conn.ConnectionState().PeerCertificates[0].Raw))
	require.NoError(t, conn.Close())

	var minted struct {
		Code      string `json:"code"`
		ExpiresAt string `json:"expiresAt"`
	}
	d.local200(http.MethodPost, "/v0/pair/code", "", &minted)
	status, raw := d.remote(http.MethodPost, "/v0/pair", "", `{"code":"`+minted.Code+`","name":"phone"}`)
	require.Equal(t, http.StatusOK, status, string(raw))
	var paired struct{ Token, ClientID string }
	d.decode(raw, &paired)
	status, _ = d.remote(http.MethodGet, "/v0/clients", "", "")
	assert.Equal(t, http.StatusUnauthorized, status, "a device without its token is refused")

	status, raw = d.remote(http.MethodGet, "/v0/clients", paired.Token, "")
	require.Equal(t, http.StatusOK, status, string(raw))
	assert.Contains(t, string(raw), "phone")
	status, _ = d.remote(http.MethodPost, "/v0/pair/code", paired.Token, "")
	assert.Equal(t, http.StatusOK, status, "a paired device can add another device")
	status, _ = d.remote(http.MethodGet, "/v0/access", paired.Token, "")
	assert.Equal(t, http.StatusOK, status, "a paired device can read the status")
	status, _ = d.remote(http.MethodPut, "/v0/access", paired.Token, `{"enabled":false}`)
	assert.Equal(t, http.StatusForbidden, status, "only the owner turns device connections off")

	d.local200(http.MethodPut, "/v0/access", `{"enabled":false}`, &st)
	assert.False(t, st.Enabled)
	_, err = d.dial()
	require.Error(t, err, "the port is closed")
	var clients []struct{ ID string }
	d.local200(http.MethodGet, "/v0/clients", "", &clients)
	require.Len(t, clients, 1, "turning it off keeps paired devices paired")

	d.local200(http.MethodPut, "/v0/access", `{"enabled":true,"address":"0.0.0.0:`+d.port+`"}`, &st)
	status, raw = d.remote(http.MethodGet, "/v0/clients", paired.Token, "")
	assert.Equal(t, http.StatusOK, status, "the same token works again: "+string(raw))

	d.local200(http.MethodDelete, "/v0/clients/"+paired.ClientID, "", nil)
	status, _ = d.remote(http.MethodGet, "/v0/clients", paired.Token, "")
	assert.Equal(t, http.StatusUnauthorized, status, "revoking a device ends its access")
}

func TestRegression_DeviceAccess_SurvivesARestart(t *testing.T) {
	h := testutil.FixtureHome(t)
	port := freePort(t)
	boot := func() (*device, func()) {
		sock := testutil.ShortSocket(t)
		c, err := internal.New(context.Background(), "unix://"+sock, nil, internal.WithHomeDir(h.Root()))
		require.NoError(t, err)
		return &device{t: t, local: testutil.UnixClient(t, sock), port: port}, c.Close
	}

	first, closeFirst := boot()
	var st accessBody
	first.local200(http.MethodPut, "/v0/access", `{"enabled":true,"address":"0.0.0.0:`+port+`"}`, &st)
	require.True(t, st.Enabled)
	closeFirst()

	second, closeSecond := boot()
	t.Cleanup(closeSecond)
	second.local200(http.MethodGet, "/v0/access", "", &st)
	assert.True(t, st.Enabled)
}
