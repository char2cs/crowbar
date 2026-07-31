//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// uiSettingsCapBytes mirrors the daemon's per-scope body cap
// (settings/handlers.MaxValueBytes). It is restated here rather than imported so
// the suite stays a black-box description of the HTTP contract: changing the cap
// in the handler must break these boundary assertions loudly, not silently
// re-target them.
const uiSettingsCapBytes = 256 << 10

func uiPath(
	scope string,
) string {
	return "/v0/settings/ui?scope=" + scope
}

// uiRequest issues one raw /v0/settings/ui call with a verbatim body and returns
// the status plus the decoded envelope, asserting nothing. It is deliberately
// assertion-free so it is safe to call from the concurrency test's goroutines,
// where a testify failure could not call t.FailNow.
func uiRequest(
	h *harness,
	method string,
	scope string,
	body []byte,
) (int, uiEnvelope, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, h.url+uiPath(scope), reader)
	if err != nil {
		return 0, uiEnvelope{}, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.server.Client().Do(req)
	if err != nil {
		return 0, uiEnvelope{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var env uiEnvelope
	if decErr := json.NewDecoder(resp.Body).Decode(&env); decErr != nil {
		return resp.StatusCode, uiEnvelope{}, decErr
	}
	return resp.StatusCode, env, nil
}

// uiEnvelope is the v0 response envelope with the UI-settings payload left as
// raw bytes, so a round-trip assertion compares what the daemon actually sent
// rather than what a Go struct decoded it into.
type uiEnvelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// getUI performs GET /v0/settings/ui?scope=… and requires a 200 success
// envelope, returning the raw stored object.
func getUI(
	t *testing.T,
	h *harness,
	scope string,
) json.RawMessage {
	t.Helper()
	status, env, err := uiRequest(h, http.MethodGet, scope, nil)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "GET %s", uiPath(scope))
	require.True(t, env.Success, "GET %s envelope error: %s", uiPath(scope), env.Error)
	return env.Data
}

// putUI performs PUT /v0/settings/ui?scope=… and requires a 200 success
// envelope, returning the raw echoed object.
func putUI(
	t *testing.T,
	h *harness,
	scope string,
	body string,
) json.RawMessage {
	t.Helper()
	status, env, err := uiRequest(h, http.MethodPut, scope, []byte(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "PUT %s", uiPath(scope))
	require.True(t, env.Success, "PUT %s envelope error: %s", uiPath(scope), env.Error)
	return env.Data
}

// putUIExpectStatus performs a PUT that is expected to FAIL, requiring the given
// status and a populated error envelope.
func putUIExpectStatus(
	t *testing.T,
	h *harness,
	scope string,
	body []byte,
	wantStatus int,
) {
	t.Helper()
	status, env, err := uiRequest(h, http.MethodPut, scope, body)
	require.NoError(t, err)
	require.Equal(t, wantStatus, status, "PUT %s with body %.60q", uiPath(scope), string(body))
	require.False(t, env.Success, "a rejected PUT must carry an error envelope")
	require.NotEmpty(t, env.Error, "a rejected PUT must explain itself")
}

// TestSettingsUI_GetBeforeAnyPut_ReturnsEmptyObject pins the first-run contract:
// a scope nothing has been written to is 200 {} and never 404. The Rust client
// boots with no local persistence at all, so "I have never saved this" and "I
// saved an empty one" have to arrive as the same answer — otherwise every read
// site in the client grows an absence special-case for a state that is not
// exceptional, it is just Tuesday.
func TestSettingsUI_GetBeforeAnyPut_ReturnsEmptyObject(t *testing.T) {
	h := newHarness(t)

	for _, scope := range []string{"global", "repo:r-unseen", "workspace:w-unseen"} {
		assert.JSONEq(t, `{}`, string(getUI(t, h, scope)),
			"an unwritten scope must read as an empty object, not a 404: %s", scope)
	}
}

// TestSettingsUI_PutThenGet_RoundTripsExactly proves the value is genuinely
// opaque. A nested object, an array of mixed types, a null, a float, a negative,
// an empty object and an empty array all have to come back exactly as sent — the
// daemon must not be quietly reshaping, coercing or dropping anything, because
// it has no idea what any of it means and no business having one.
func TestSettingsUI_PutThenGet_RoundTripsExactly(t *testing.T) {
	h := newHarness(t)

	const layout = `{
		"rootLayout": {
			"type": "split",
			"direction": "horizontal",
			"children": [
				{"type": "pane", "id": "p1", "ratio": 0.5},
				{"type": "pane", "id": "p2", "ratio": 0.5}
			]
		},
		"buffers": [
			{"paneId": "p1", "path": "api/main.go", "pinned": true},
			{"paneId": "p2", "path": "web/src/app.tsx", "pinned": false}
		],
		"activePaneId": "p1",
		"sidebarWidth": 242.5,
		"scrollOffset": -13,
		"lastClosed": null,
		"flags": {},
		"recent": [],
		"title": "café ✓ \"quoted\""
	}`

	echoed := putUI(t, h, "workspace:w1", layout)
	assert.JSONEq(t, layout, string(echoed),
		"PUT must echo the object it stored so a client reconciles from server truth without a second read")

	assert.JSONEq(t, layout, string(getUI(t, h, "workspace:w1")),
		"GET must return the stored object verbatim, nested values and all")
}

// TestSettingsUI_PutReplacesWholesale proves PUT is a replace and not a merge.
// The client owns the whole blob; a key it stopped writing is a key it deleted,
// and a daemon that helpfully merged would make removing a setting impossible.
func TestSettingsUI_PutReplacesWholesale(t *testing.T) {
	h := newHarness(t)

	putUI(t, h, "global", `{"theme":"dark","fontSize":13,"minimap":true,"wordWrap":false}`)
	putUI(t, h, "global", `{"theme":"light"}`)

	assert.JSONEq(t, `{"theme":"light"}`, string(getUI(t, h, "global")),
		"the second PUT replaces the stored value wholesale: keys it omitted must be GONE, not merged forward")
}

// TestSettingsUI_ScopeIsolation proves the scope key is a real partition. The
// three forms are not decoration: "global" holds machine-wide preferences,
// "repo:<id>" the per-repo workspace hierarchy, and "workspace:<id>" the pane
// layout, and one workspace's layout leaking into another's would be visible as
// windows rearranging themselves on tab switch.
func TestSettingsUI_ScopeIsolation(t *testing.T) {
	h := newHarness(t)

	scopes := map[string]string{
		"global":       `{"theme":"dark"}`,
		"repo:r1":      `{"entries":[{"wsId":"w1"}]}`,
		"repo:r2":      `{"entries":[{"wsId":"w9","parentId":"w1"}]}`,
		"workspace:w1": `{"activePaneId":"p1"}`,
		"workspace:w2": `{"activePaneId":"p7"}`,
	}
	for scope, body := range scopes {
		putUI(t, h, scope, body)
	}

	for scope, body := range scopes {
		assert.JSONEq(t, body, string(getUI(t, h, scope)),
			"scope %s must still hold its own value after every other scope was written", scope)
	}

	assert.JSONEq(t, `{}`, string(getUI(t, h, "workspace:w3")),
		"a scope nobody wrote must stay empty no matter how many neighbours were written")
}

// TestSettingsUI_NonObjectBody_Rejected400 pins the one shape rule the daemon
// does enforce. The value is opaque but it is not arbitrary: it is a JSON
// OBJECT, so a client can always merge-on-read and so the stored bytes can never
// be a bare scalar that a future GET would have to guess the type of.
func TestSettingsUI_NonObjectBody_Rejected400(t *testing.T) {
	h := newHarness(t)

	for _, body := range []string{
		`[1,2,3]`,
		`[]`,
		`"a string"`,
		`42`,
		`true`,
		`null`,
		``,
		`{"unterminated": `,
		`{"a":1}{"b":2}`,
		`not json at all`,
	} {
		putUIExpectStatus(t, h, "global", []byte(body), http.StatusBadRequest)
	}

	assert.JSONEq(t, `{}`, string(getUI(t, h, "global")),
		"a rejected PUT must not have written anything")
}

// TestSettingsUI_InvalidScope_Rejected400 keeps the table from becoming a junk
// drawer. Only the three known forms are addressable, so a client typo lands as
// a loud 400 instead of a row nothing will ever read again.
func TestSettingsUI_InvalidScope_Rejected400(t *testing.T) {
	h := newHarness(t)

	for _, scope := range []string{
		"",
		"globals",
		"workspace",
		"workspace:",
		"repo:",
		"project:p1",
		"workspace:w1/../global",
		"workspace:" + strings.Repeat("w", 65),
	} {
		putUIExpectStatus(t, h, scope, []byte(`{"a":1}`), http.StatusBadRequest)

		status, env, err := uiRequest(h, http.MethodGet, scope, nil)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, status, "GET must reject scope %q too", scope)
		assert.False(t, env.Success)
	}
}

// TestSettingsUI_BodySizeCap proves the cap is enforced exactly where it claims
// to be: a body of exactly 256 KiB is stored and round-trips intact, and one
// byte more is refused 413. The upper bound matters because the value lands in
// the daemon's GLOBAL view.db on a debounced save loop — a client that started
// shipping document text in here would otherwise write megabytes per keystroke
// burst into the database every other read on the machine shares.
func TestSettingsUI_BodySizeCap(t *testing.T) {
	h := newHarness(t)

	const envelopeOverhead = len(`{"blob":""}`)

	atCap := fmt.Sprintf(`{"blob":%q}`, strings.Repeat("a", uiSettingsCapBytes-envelopeOverhead))
	require.Len(t, atCap, uiSettingsCapBytes, "the at-cap fixture must be exactly the cap")
	putUI(t, h, "workspace:w1", atCap)
	assert.JSONEq(t, atCap, string(getUI(t, h, "workspace:w1")),
		"a body of exactly the cap must be stored whole, not truncated by the limit reader")

	overCap := fmt.Sprintf(`{"blob":%q}`, strings.Repeat("a", uiSettingsCapBytes-envelopeOverhead+1))
	require.Len(t, overCap, uiSettingsCapBytes+1)
	putUIExpectStatus(t, h, "workspace:w1", []byte(overCap), http.StatusRequestEntityTooLarge)

	assert.JSONEq(t, atCap, string(getUI(t, h, "workspace:w1")),
		"the 413 must leave the previously stored value untouched")
}

// TestSettingsUI_OversizeChunkedBody_Rejected413 covers the same cap when the
// request declares no Content-Length. A cap checked only against the declared
// length is not a cap — it is a suggestion the client is free to decline — so
// the read itself has to be bounded.
func TestSettingsUI_OversizeChunkedBody_Rejected413(t *testing.T) {
	h := newHarness(t)

	payload := fmt.Sprintf(`{"blob":%q}`, strings.Repeat("b", uiSettingsCapBytes))
	req, err := http.NewRequest(
		http.MethodPut,
		h.url+uiPath("global"),
		io.NopCloser(strings.NewReader(payload)),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	// A negative ContentLength with a non-nil Body is net/http's "length
	// unknown": the transport streams the body chunked and the server sees
	// ContentLength -1, so the handler's declared-length short circuit cannot
	// fire and only the bounded read can refuse this.
	req.ContentLength = -1

	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode,
		"an oversize body must be refused on the wire even when it declares no length")
}

// TestSettingsUI_ConcurrentPutsSameScope_LastWriterWinsIntact is the
// interleaving proof. Every writer targets the SAME scope at once and every one
// of them must be accepted; afterwards the stored value must be exactly ONE of
// the submitted objects, whole. A torn or spliced result, or a 500 from two
// first-writes racing the same insert, would show up here as a value matching
// none of them.
//
// The goroutines are released by a barrier and joined by a WaitGroup, so the
// test blocks on real completion signals and never on the clock.
func TestSettingsUI_ConcurrentPutsSameScope_LastWriterWinsIntact(t *testing.T) {
	h := newHarness(t)

	const writers = 24
	const scope = "workspace:w-contended"

	bodies := make([]string, writers)
	for i := range bodies {
		bodies[i] = fmt.Sprintf(
			`{"writer":%d,"activePaneId":"p%d","buffers":["a%d","b%d"],"nested":{"depth":%d}}`,
			i, i, i, i, i,
		)
	}

	statuses := make([]int, writers)
	errs := make([]error, writers)

	var release sync.WaitGroup
	release.Add(1)
	var joined sync.WaitGroup

	for i := range bodies {
		joined.Add(1)
		go func(idx int) {
			defer joined.Done()
			release.Wait()
			statuses[idx], _, errs[idx] = uiRequest(h, http.MethodPut, scope, []byte(bodies[idx]))
		}(i)
	}

	release.Done()
	joined.Wait()

	for i := range bodies {
		require.NoError(t, errs[i], "writer %d transport error", i)
		require.Equal(t, http.StatusOK, statuses[i],
			"every concurrent PUT to one scope must be accepted; writer %d was not", i)
	}

	stored := string(getUI(t, h, scope))
	matched := false
	for _, body := range bodies {
		if jsonEquivalent(t, body, stored) {
			matched = true
			break
		}
	}
	assert.Truef(t, matched,
		"after %d concurrent PUTs the stored value must be one submitted object INTACT, got: %s",
		writers, stored)
}

func jsonEquivalent(
	t *testing.T,
	want string,
	got string,
) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal([]byte(want), &a); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(got), &b); err != nil {
		return false
	}
	return assert.ObjectsAreEqual(a, b)
}

// TestSettingsUI_SurvivesDaemonRestart is the reason this endpoint exists at
// all. The Rust-native client keeps NO local state, so if the value is not in
// view.db then every daemon restart is a fresh install: panes reset, sidebar
// re-expands, theme reverts. A second daemon booted over the same home must hand
// back exactly what the first one was given.
func TestSettingsUI_SurvivesDaemonRestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)

	const globalPrefs = `{"theme":"dark","fontSize":13,"fontFamily":"Berkeley Mono"}`
	const repoTree = `{"entries":[{"wsId":"w1"},{"wsId":"w2","parentId":"w1"}]}`
	const wsLayout = `{"activePaneId":"p2","panes":{"p1":{"buffers":["README.md"]}}}`

	putUI(t, h1, "global", globalPrefs)
	putUI(t, h1, "repo:r1", repoTree)
	putUI(t, h1, "workspace:w1", wsLayout)

	h1.shutdown()

	h2 := newHarnessAt(t, home)

	assert.JSONEq(t, globalPrefs, string(getUI(t, h2, "global")),
		"global UI preferences must outlive the daemon that stored them")
	assert.JSONEq(t, repoTree, string(getUI(t, h2, "repo:r1")),
		"the per-repo scope must outlive the daemon that stored it")
	assert.JSONEq(t, wsLayout, string(getUI(t, h2, "workspace:w1")),
		"the per-workspace layout must outlive the daemon that stored it")
	assert.JSONEq(t, `{}`, string(getUI(t, h2, "workspace:w2")),
		"a restart must not conjure state for a scope nothing ever wrote")
}

// TestSettingsUI_ReplaceSurvivesDaemonRestart guards the pairing of the two
// rules above: a wholesale replace must be what PERSISTS, so a key the client
// deleted does not quietly return on the next boot.
func TestSettingsUI_ReplaceSurvivesDaemonRestart(t *testing.T) {
	home := t.TempDir()
	h1 := newHarnessAt(t, home)

	putUI(t, h1, "global", `{"theme":"dark","minimap":true,"fontSize":13}`)
	putUI(t, h1, "global", `{"theme":"light"}`)
	h1.shutdown()

	h2 := newHarnessAt(t, home)

	assert.JSONEq(t, `{"theme":"light"}`, string(getUI(t, h2, "global")),
		"the replace is what is durable: a dropped key must not come back after a restart")
}
