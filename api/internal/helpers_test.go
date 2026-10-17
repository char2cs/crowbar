package internal_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
