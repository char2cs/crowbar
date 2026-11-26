package internal_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// daemon is a daemon booted on a socket of its own, and a client of it.
type daemon struct {
	*internal.Container
	client *http.Client
}

// boot runs internal.New on h over a socket of its own. The daemon it returns
// closes with the test.
func boot(
	ctx context.Context,
	t *testing.T,
	h home.Home,
) (daemon, error) {
	t.Helper()
	sock := testutil.ShortSocket(t)
	c, err := internal.New(ctx, "unix://"+sock, nil, internal.WithHomeDir(h.Root()))
	if c != nil {
		t.Cleanup(c.Close)
	}
	return daemon{Container: c, client: testutil.UnixClient(t, sock)}, err
}

// envelope is a gate answer or a health answer, decoded.
type envelope struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
	Data    struct {
		Status string          `json:"status"`
		PID    int             `json:"pid"`
		Detail json.RawMessage `json:"detail"`
	} `json:"data"`
}

// answer is one response of the daemon.
type answer struct {
	status int
	header http.Header
	raw    string
	envelope
}

func call(
	t *testing.T,
	client *http.Client,
	method string,
	path string,
	header http.Header,
) answer {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, "http://crowbar"+path, http.NoBody)
	require.NoError(t, err)
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	a := answer{status: resp.StatusCode, header: resp.Header, raw: string(raw)}
	if len(raw) > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		require.NoError(t, json.Unmarshal(raw, &a.envelope), string(raw))
	}
	return a
}

func (d daemon) get(
	t *testing.T,
	path string,
) answer {
	t.Helper()
	return call(t, d.client, http.MethodGet, path, nil)
}

// detail decodes a health answer's detail as T.
func detail[T any](
	t *testing.T,
	a answer,
) T {
	t.Helper()
	var out T
	require.NoError(t, json.Unmarshal(a.Data.Detail, &out), a.raw)
	return out
}

// ledgerRecord is what a test reads of one ledger line.
type ledgerRecord struct {
	Kind    string `json:"kind"`
	Step    int    `json:"step,omitempty"`
	Name    string `json:"name,omitempty"`
	Outcome string `json:"outcome,omitempty"`
}

func readLedger(
	t *testing.T,
	h home.Home,
) []ledgerRecord {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.MigrateDir(), "ledger.jsonl"))
	require.NoError(t, err)
	var out []ledgerRecord
	lines := bufio.NewScanner(bytes.NewReader(raw))
	for lines.Scan() {
		var r ledgerRecord
		require.NoError(t, json.Unmarshal(lines.Bytes(), &r))
		out = append(out, r)
	}
	require.NoError(t, lines.Err())
	return out
}

// requireHomeFree fails unless h can be taken: no daemon holds it any more.
func requireHomeFree(
	t *testing.T,
	h home.Home,
) {
	t.Helper()
	sess, err := migrations.Begin(t.Context(), h, migrate.Options{})
	require.NoError(t, err, "the home is free")
	require.NoError(t, sess.Close())
}

// labelled reports whether any running goroutine carries value under key; a
// goroutine inherits the labels of the one that started it.
func labelled(
	t *testing.T,
	key string,
	value string,
) bool {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, pprof.Lookup("goroutine").WriteTo(&buf, 1))
	return strings.Contains(buf.String(), fmt.Sprintf("%q:%q", key, value))
}

// device drives a real daemon the way the desktop and a phone do: the local
// socket for the owner, TLS and a bearer for another device.
type device struct {
	t     *testing.T
	local *http.Client
	tls   *http.Client
	port  string
}

func (d *device) do(client *http.Client, base, method, path, bearer, body string) (int, []byte) {
	d.t.Helper()
	req, err := http.NewRequestWithContext(d.t.Context(), method, base+path, strings.NewReader(body))
	require.NoError(d.t, err)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(d.t, err)
	return resp.StatusCode, raw
}

func (d *device) local200(method, path, body string, out any) {
	d.t.Helper()
	code, raw := d.do(d.local, "http://daemon", method, path, "", body)
	require.Equal(d.t, http.StatusOK, code, string(raw))
	d.decode(raw, out)
}

func (d *device) remote(method, path, bearer, body string) (int, []byte) {
	d.t.Helper()
	d.tls.CloseIdleConnections()
	return d.do(d.tls, "https://127.0.0.1:"+d.port, method, path, bearer, body)
}

// dial opens a TLS connection to the device port, trusting any certificate: the tests compare
// its fingerprint to the one the daemon reports instead.
func (d *device) dial() (*tls.Conn, error) {
	conn, err := (&tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}).DialContext(d.t.Context(), "tcp", "127.0.0.1:"+d.port)
	if err != nil {
		return nil, err
	}
	tlsConn, ok := conn.(*tls.Conn)
	require.True(d.t, ok)
	return tlsConn, nil
}

func (d *device) decode(raw []byte, out any) {
	d.t.Helper()
	if out == nil {
		return
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(d.t, json.Unmarshal(raw, &env), string(raw))
	require.NoError(d.t, json.Unmarshal(env.Data, out), string(raw))
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	return port
}

type accessBody struct {
	Enabled     bool   `json:"enabled"`
	Fingerprint string `json:"fingerprint"`
}
