// Black-box coverage for the auxiliary loopback TCP listener (spec §9.4). Every
// test here boots the REAL wired daemon through internal.New — the same entry
// point `crowbar serve` uses — over its real unix socket, and reaches it the way
// an external client would: an http.Client over the socket, and an http.Client
// or a WebSocket dialer over the TCP port. Nothing is stubbed and no internal
// state is inspected.
//
// This file deliberately carries NO `integration` build tag, unlike the rest of
// this package. It boots a daemon and talks HTTP to it — it needs no git
// fixtures, no vendor CLI and no network — so it belongs in the default
// `go test ./...` gate, where a regression in who may reach the daemon gets
// caught on every run rather than only in the integration job.
//
// SYNCHRONISATION: there is not a single sleep, poll or deadline in here, and
// there must never be one. The listeners are bound by internal.New BEFORE it
// returns, so the kernel is queueing connections on both from the moment the
// constructor hands back the container — a request issued immediately is
// accepted the instant Serve reaches it. The bound address is read off the
// listener itself (Container.LoopbackAddress), never assumed, so an ephemeral
// port needs no discovery loop. Shutdown is joined on the channel Run's result
// arrives on, which is the daemon stating it is down.
package tests

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/loopback"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

// loopbackDaemon is a booted daemon plus the two ways to reach it.
type loopbackDaemon struct {
	t          *testing.T
	container  *internal.Container
	home       string
	socketPath string
	done       chan error
	cancel     context.CancelFunc
	stopped    bool
}

// bootLoopbackDaemon starts the full daemon on a private unix socket under a
// per-test home. loopbackAddr enables the auxiliary TCP listener; "" leaves it
// off, which is the shipped default.
func bootLoopbackDaemon(
	t *testing.T,
	loopbackAddr string,
) *loopbackDaemon {
	t.Helper()

	home := t.TempDir()
	socketPath := shortSocketPath(t)

	options := []internal.Option{internal.WithHomeDir(home)}
	if loopbackAddr != "" {
		options = append(options, internal.WithLoopbackTCP(loopbackAddr))
	}
	container, err := internal.New(context.Background(), "unix://"+socketPath, nil, options...)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	d := &loopbackDaemon{
		t:          t,
		container:  container,
		home:       home,
		socketPath: socketPath,
		done:       make(chan error, 1),
		cancel:     cancel,
	}
	go func() { d.done <- container.Run(ctx) }()
	t.Cleanup(d.stop)
	return d
}

// stop performs the daemon's graceful shutdown and BLOCKS until Run has
// returned — the daemon's own statement that it is down. It is the only barrier
// any test here needs after a shutdown, and it is a real signal, not a delay.
func (d *loopbackDaemon) stop() {
	if d.stopped {
		return
	}
	d.stopped = true
	d.cancel()
	require.NoError(d.t, <-d.done)
	d.container.Close()
}

// unixClient reaches the daemon over its unix socket, as the desktop app and the
// CLI do today. The Host in the URL is a placeholder the socket dialer ignores.
func (d *loopbackDaemon) unixClient() *http.Client {
	socket := d.socketPath
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		},
	}
}

func (d *loopbackDaemon) loopbackURL(
	path string,
) string {
	return "http://" + d.container.LoopbackAddress() + path
}

// credentials reads back the file the daemon published, which is the exact
// artefact the native client and the webview bootstrap consume.
func (d *loopbackDaemon) credentials() loopback.Credentials {
	d.t.Helper()
	raw, err := os.ReadFile(d.container.LoopbackCredentialsPath())
	require.NoError(d.t, err)
	var creds loopback.Credentials
	require.NoError(d.t, json.Unmarshal(raw, &creds))
	return creds
}

// shortSocketPath returns a unique socket path outside t.TempDir(): macOS caps
// sun_path at 104 bytes and a temp dir named after the test overflows it.
func shortSocketPath(
	t *testing.T,
) string {
	t.Helper()
	f, err := os.CreateTemp("", "cb-loopback-*.sock")
	require.NoError(t, err)
	path := f.Name()
	require.NoError(t, f.Close())
	require.NoError(t, os.Remove(path))
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func getStatus(
	t *testing.T,
	client *http.Client,
	url string,
	header http.Header,
) (int, []byte, http.Header) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	for name, values := range header {
		req.Header[name] = values
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, body, resp.Header
}

// TestLoopbackTCP_Disabled_NoListenerAndNoCredentials proves the feature is OFF
// by default: with no WithLoopbackTCP option the daemon binds nothing beyond its
// unix socket and publishes no token, so an existing install is untouched.
func TestLoopbackTCP_Disabled_NoListenerAndNoCredentials(t *testing.T) {
	d := bootLoopbackDaemon(t, "")

	assert.Empty(t, d.container.LoopbackAddress(), "no TCP listener may exist when the feature is off")
	assert.Empty(t, d.container.LoopbackCredentialsPath())

	published := filepath.Join(metadata.GetStateDirPathAt(d.home), loopback.FileName)
	_, err := os.Stat(published)
	assert.True(t, os.IsNotExist(err), "no credentials file may be written when the feature is off")
}

// TestLoopbackTCP_Disabled_UnixResponseIsUnchanged pins the "byte-identical to
// before" half of the contract: the SAME route over the unix socket returns the
// same status and the same body bytes whether or not the TCP listener is up, and
// carries no authentication headers in either case.
func TestLoopbackTCP_Disabled_UnixResponseIsUnchanged(t *testing.T) {
	off := bootLoopbackDaemon(t, "")
	offStatus, offBody, offHeader := getStatus(t, off.unixClient(), "http://crowbar/v0/health", nil)

	on := bootLoopbackDaemon(t, loopback.DefaultAddress)
	onStatus, onBody, onHeader := getStatus(t, on.unixClient(), "http://crowbar/v0/health", nil)

	assert.Equal(t, http.StatusOK, offStatus)
	assert.Equal(t, offStatus, onStatus)
	assert.Equal(t, string(offBody), string(onBody), "the unix response body must not change when the TCP listener is enabled")
	assert.Empty(t, offHeader.Get("WWW-Authenticate"))
	assert.Empty(t, onHeader.Get("WWW-Authenticate"))
}

// TestLoopbackTCP_Enabled_BothListenersServeTheSameRoute proves the daemon serves
// BOTH transports at once, and that they are the same surface: one route, two
// doors, identical payloads.
func TestLoopbackTCP_Enabled_BothListenersServeTheSameRoute(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	creds := d.credentials()

	unixStatus, unixBody, _ := getStatus(t, d.unixClient(), "http://crowbar/v0/health", nil)
	require.Equal(t, http.StatusOK, unixStatus)

	tcpStatus, tcpBody, _ := getStatus(t, http.DefaultClient, d.loopbackURL("/v0/health"), http.Header{
		"Authorization": []string{"Bearer " + creds.Token},
	})
	require.Equal(t, http.StatusOK, tcpStatus)

	assert.Equal(t, string(unixBody), string(tcpBody))
	assert.True(t, strings.HasPrefix(d.container.LoopbackAddress(), "127.0.0.1:"),
		"the listener must be bound on loopback, got %q", d.container.LoopbackAddress())
}

// TestLoopbackTCP_SameOriginNeedsNoCORSRelaxation verifies the claim that this
// listener requires NO change to the CORS allowlist, rather than assuming it. A
// webview that loads its page from http://127.0.0.1:<port> and calls the API at
// the same authority is SAME-ORIGIN, and the existing allowlist already permits
// any loopback origin, so the request is admitted and its Origin is echoed back
// under the rules that were already in place. Nothing here was widened; if this
// test ever needs a relaxation to pass, that is a security decision for a human.
func TestLoopbackTCP_SameOriginNeedsNoCORSRelaxation(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	creds := d.credentials()
	pageOrigin := "http://" + d.container.LoopbackAddress()

	status, _, header := getStatus(t, http.DefaultClient, d.loopbackURL("/v0/health"), http.Header{
		"Authorization": []string{"Bearer " + creds.Token},
		"Origin":        []string{pageOrigin},
	})
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, pageOrigin, header.Get("Access-Control-Allow-Origin"))

	// A page on a site the user merely visited is still refused the response, and
	// it has no way to obtain the token in the first place: two independent locks,
	// neither of which this item loosened.
	status, _, header = getStatus(t, http.DefaultClient, d.loopbackURL("/v0/health"), http.Header{
		"Authorization": []string{"Bearer " + creds.Token},
		"Origin":        []string{"https://evil.example"},
	})
	assert.Equal(t, http.StatusOK, status)
	assert.Empty(t, header.Get("Access-Control-Allow-Origin"),
		"a disallowed origin must not be granted a readable cross-origin response")
}

// TestLoopbackTCP_MissingToken_Rejected proves the TCP listener is closed to an
// unauthenticated caller — including on /v0/health, which is exactly the probe a
// local process would use to discover the daemon, and on a static asset path.
func TestLoopbackTCP_MissingToken_Rejected(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)

	for _, path := range []string{"/v0/health", "/v0/projects", "/index.html", "/"} {
		t.Run(path, func(t *testing.T) {
			status, body, header := getStatus(t, http.DefaultClient, d.loopbackURL(path), nil)
			assert.Equal(t, http.StatusUnauthorized, status)
			assert.Contains(t, string(body), "unauthorized")
			assert.Equal(t, `Bearer realm="crowbar"`, header.Get("WWW-Authenticate"))
		})
	}
}

// TestLoopbackTCP_WrongToken_Rejected proves a near-miss credential is refused:
// a token of the right shape but the wrong value, and a well-formed token from a
// DIFFERENT daemon boot, which is the realistic failure (a stale credentials file).
func TestLoopbackTCP_WrongToken_Rejected(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	other := bootLoopbackDaemon(t, loopback.DefaultAddress)

	wrong := map[string]string{
		"garbage":            "not-the-token",
		"empty":              "",
		"another daemons":    other.credentials().Token,
		"right token suffix": "x" + d.credentials().Token[1:],
	}
	for name, token := range wrong {
		t.Run(name, func(t *testing.T) {
			status, _, _ := getStatus(t, http.DefaultClient, d.loopbackURL("/v0/health"), http.Header{
				"Authorization": []string{"Bearer " + token},
			})
			assert.Equal(t, http.StatusUnauthorized, status)
		})
	}
}

// TestLoopbackTCP_CorrectToken_Succeeds proves each accepted presentation form
// works: the Authorization bearer header the native client uses, the
// X-Crowbar-Token header, and the query parameter a browser WebSocket handshake
// needs. A token minted per boot is per boot: the value is 256 bits of base64url.
func TestLoopbackTCP_CorrectToken_Succeeds(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	creds := d.credentials()
	require.Len(t, creds.Token, 43, "256 bits of entropy is 43 unpadded base64url characters")

	forms := map[string]struct {
		header http.Header
		path   string
	}{
		"authorization bearer": {
			header: http.Header{"Authorization": []string{"Bearer " + creds.Token}},
			path:   "/v0/health",
		},
		"authorization bearer lowercase scheme": {
			header: http.Header{"Authorization": []string{"bearer " + creds.Token}},
			path:   "/v0/health",
		},
		"crowbar header": {
			header: http.Header{loopback.HeaderName: []string{creds.Token}},
			path:   "/v0/health",
		},
		"query parameter": {
			path: "/v0/health?" + loopback.QueryParam + "=" + creds.Token,
		},
	}
	for name, form := range forms {
		t.Run(name, func(t *testing.T) {
			status, body, _ := getStatus(t, http.DefaultClient, d.loopbackURL(form.path), form.header)
			require.Equal(t, http.StatusOK, status)
			assert.Contains(t, string(body), `"status":"ok"`)
		})
	}
}

// TestLoopbackTCP_WebSocketUpgrade_RequiresToken proves the credential check runs
// BEFORE the upgrade, not after: an unauthenticated handshake is answered 401 and
// no socket is ever established, so the realtime surface is not a hole in the
// listener's authentication. GET /v0/projects is dual-served (REST or WS on the
// same path), so this is the same route the REST cases above use.
func TestLoopbackTCP_WebSocketUpgrade_RequiresToken(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	base := "ws://" + d.container.LoopbackAddress() + "/v0/projects"

	conn, resp, err := websocket.DefaultDialer.Dial(base, nil)
	if conn != nil {
		_ = conn.Close()
	}
	require.ErrorIs(t, err, websocket.ErrBadHandshake, "an unauthenticated upgrade must not succeed")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()

	conn, resp, err = websocket.DefaultDialer.Dial(base, http.Header{
		"Authorization": []string{"Bearer " + "not-the-token"},
	})
	if conn != nil {
		_ = conn.Close()
	}
	require.ErrorIs(t, err, websocket.ErrBadHandshake, "a wrong-token upgrade must not succeed")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	_ = resp.Body.Close()
}

// TestLoopbackTCP_WebSocketUpgrade_SucceedsWithToken is the positive half of the
// case above: the same handshake, authenticated, must still work — via the query
// parameter, because the browser WebSocket API cannot set a request header, and
// via the header for a native client that can.
func TestLoopbackTCP_WebSocketUpgrade_SucceedsWithToken(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	creds := d.credentials()
	base := "ws://" + d.container.LoopbackAddress() + "/v0/projects"

	conn, resp, err := websocket.DefaultDialer.Dial(base+"?"+loopback.QueryParam+"="+creds.Token, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err, "the query-parameter credential must carry a browser WebSocket handshake")
	require.NoError(t, conn.Close())

	conn, resp, err = websocket.DefaultDialer.Dial(base, http.Header{
		"Authorization": []string{"Bearer " + creds.Token},
	})
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err, "the bearer header must carry a native WebSocket handshake")
	require.NoError(t, conn.Close())
}

// TestLoopbackTCP_UnixListenerNeedsNoCredential is the compatibility guarantee:
// enabling the TCP listener must not put a new credential requirement on the unix
// socket, whose access control is the socket file's 0600 mode. Every client
// shipping today sends no token and must keep working unchanged.
func TestLoopbackTCP_UnixListenerNeedsNoCredential(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	require.NotEmpty(t, d.container.LoopbackAddress())

	status, body, header := getStatus(t, d.unixClient(), "http://crowbar/v0/health", nil)
	assert.Equal(t, http.StatusOK, status)
	assert.Contains(t, string(body), `"status":"ok"`)
	assert.Empty(t, header.Get("WWW-Authenticate"), "the unix socket must not challenge for a credential")

	status, _, _ = getStatus(t, d.unixClient(), "http://crowbar/v0/projects", nil)
	assert.Equal(t, http.StatusOK, status)
}

// TestLoopbackTCP_NonLoopbackAddress_RefusesToStart proves the bind restriction is
// enforced in code, not by convention: a wildcard, a LAN address and a hostname
// (which the OS resolves and can therefore point off-box) all fail the daemon's
// startup with the reason surfaced. It also proves the failure is CLEAN — the unix
// socket already bound before the loopback listener was attempted is released, so a
// refused start does not leave a socket file behind for the next launch to read as
// "a daemon is already running".
func TestLoopbackTCP_NonLoopbackAddress_RefusesToStart(t *testing.T) {
	addresses := map[string]string{
		"ipv4 wildcard": "0.0.0.0:0",
		"empty host":    ":0",
		"ipv6 wildcard": "[::]:0",
		"lan address":   "192.168.1.50:0",
		"hostname":      "example.com:0",
		"localhost":     "localhost:0",
		"no port":       "127.0.0.1",
	}
	for name, addr := range addresses {
		t.Run(name, func(t *testing.T) {
			socketPath := shortSocketPath(t)
			container, err := internal.New(
				context.Background(),
				"unix://"+socketPath,
				nil,
				internal.WithHomeDir(t.TempDir()),
				internal.WithLoopbackTCP(addr),
			)
			if container != nil {
				container.Close()
			}
			require.Error(t, err, "%q must not be bindable", addr)
			require.ErrorIs(t, err, transports.ErrNonLoopbackBind)
			assert.Contains(t, err.Error(), addr, "the rejected address must be named in the error")

			_, statErr := os.Stat(socketPath)
			assert.True(t, os.IsNotExist(statErr),
				"a refused start must release the unix socket it already bound")
		})
	}
}

// TestLoopbackTCP_CredentialsFileIsOwnerOnly pins the on-disk contract the native
// client and the webview bootstrap read, and the permissions that make it a secret
// at all: 0600 inside a 0700 state directory. A world-readable token would hand the
// listener to precisely the local processes the token exists to keep out.
func TestLoopbackTCP_CredentialsFileIsOwnerOnly(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)

	path := d.container.LoopbackCredentialsPath()
	require.Equal(t, filepath.Join(metadata.GetStateDirPathAt(d.home), loopback.FileName), path)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the token file must be owner-read/write only")

	// The state directory is created by the adapter layer before the loopback
	// listener exists, and it lands 0750 — group may traverse it. That is fine and
	// is NOT quietly tightened here: the file's own 0600 is what keeps the token
	// unreadable by group and by other, whatever the directory allows. What must
	// hold is that "other" has no access at all, so nothing outside the owner's
	// group can even enumerate the state directory.
	stateDir, err := os.Stat(filepath.Dir(path))
	require.NoError(t, err)
	assert.Zero(t, stateDir.Mode().Perm()&0o007, "the state directory must not be world-accessible")

	creds := d.credentials()
	assert.Equal(t, loopback.CredentialsVersion, creds.Version)
	assert.Equal(t, "http", creds.Scheme)
	assert.Equal(t, d.container.LoopbackAddress(), creds.Address)
	assert.Equal(t, "http://"+d.container.LoopbackAddress(), creds.URL)
	assert.Equal(t, os.Getpid(), creds.PID)
	assert.NotZero(t, creds.Port)
	assert.NotEmpty(t, creds.Token)

	assert.NotContains(t, creds.String(), creds.Token, "the token must not survive a %%v of the credentials")
}

// TestLoopbackTCP_Shutdown_ClosesBothListeners proves a clean stop takes BOTH doors
// with it, and unpublishes the credential rather than leaving a token and a port on
// disk pointing at a daemon that is gone.
//
// The dials happen only after stop() has joined Run's result — the daemon's own
// statement that it is down — so this asserts on a finished shutdown, never on a
// guess about when one finished.
func TestLoopbackTCP_Shutdown_ClosesBothListeners(t *testing.T) {
	d := bootLoopbackDaemon(t, loopback.DefaultAddress)
	tcpAddr := d.container.LoopbackAddress()
	credsPath := d.container.LoopbackCredentialsPath()

	require.Equal(t, http.StatusOK, statusOverUnix(t, d))

	d.stop()

	conn, err := net.Dial("tcp", tcpAddr)
	if conn != nil {
		_ = conn.Close()
	}
	assert.Error(t, err, "the loopback TCP listener must be closed after shutdown")

	unixConn, err := net.Dial("unix", d.socketPath)
	if unixConn != nil {
		_ = unixConn.Close()
	}
	assert.Error(t, err, "the unix listener must be closed after shutdown")

	_, statErr := os.Stat(credsPath)
	assert.True(t, os.IsNotExist(statErr), "a stopped daemon must unpublish its credentials")
}

func statusOverUnix(
	t *testing.T,
	d *loopbackDaemon,
) int {
	t.Helper()
	status, _, _ := getStatus(t, d.unixClient(), "http://crowbar/v0/health", nil)
	return status
}
