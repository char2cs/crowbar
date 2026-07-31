package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/settings"
	settingshandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/settings/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// memStore is an in-memory UISettingsStore whose failure modes and concurrency
// are directly observable, standing in for the sqlite-backed store.
type memStore struct {
	mu     sync.Mutex
	rows   map[string]string
	getErr error
	putErr error

	inFlight    map[string]int32
	maxOverlap  int32
	trackScopes bool

	// rendezvous, when set, makes every Save wait until the expected number of
	// Saves are inside the store at once. It is how the cross-scope test proves
	// two scopes can be written concurrently.
	rendezvous *sync.WaitGroup
}

func newMemStore() *memStore {
	return &memStore{
		rows:     map[string]string{},
		inFlight: map[string]int32{},
	}
}

func (s *memStore) FindByKey(
	_ context.Context,
	scope string,
) (*domain.UISettings, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.rows[scope]
	if !ok {
		return nil, nil
	}
	return &domain.UISettings{Scope: scope, Value: value}, nil
}

func (s *memStore) Save(
	_ context.Context,
	item domain.UISettings,
) error {
	if s.putErr != nil {
		return s.putErr
	}
	if s.trackScopes {
		s.enter(item.Scope)
		defer s.leave(item.Scope)
	}
	if s.rendezvous != nil {
		s.rendezvous.Done()
		s.rendezvous.Wait()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[item.Scope] = item.Value
	return nil
}

// enter records that a Save for one scope is in progress and yields the
// processor several times, so any handler that did NOT serialise same-scope
// writes would be observed with two Saves overlapping.
func (s *memStore) enter(
	scope string,
) {
	s.mu.Lock()
	s.inFlight[scope]++
	current := s.inFlight[scope]
	s.mu.Unlock()

	if current > atomic.LoadInt32(&s.maxOverlap) {
		atomic.StoreInt32(&s.maxOverlap, current)
	}
	for i := 0; i < 8; i++ {
		runtime.Gosched()
	}
}

func (s *memStore) leave(
	scope string,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inFlight[scope]--
}

func newServer(
	t *testing.T,
	store settingshandlers.UISettingsStore,
) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	settings.Register(r.Group("/v0"), store)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func call(
	t *testing.T,
	srv *httptest.Server,
	method string,
	scope string,
	body string,
) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, srv.URL+"/v0/settings/ui?scope="+scope, reader)
	require.NoError(t, err)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(raw)
}

func dataOf(
	t *testing.T,
	payload string,
) string {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(payload), &env))
	require.True(t, env.Success, "expected a success envelope, got %s", payload)
	return string(env.Data)
}

func TestGetUI_AbsentScope_Returns200EmptyObject(t *testing.T) {
	srv := newServer(t, newMemStore())

	status, body := call(t, srv, http.MethodGet, "global", "")

	assert.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{}`, dataOf(t, body))
}

func TestPutUI_StoresCompactedAndEchoes(t *testing.T) {
	store := newMemStore()
	srv := newServer(t, store)

	status, body := call(t, srv, http.MethodPut, "workspace:w1", "{\n  \"a\" : [1, 2],\n  \"b\": {\"c\": true}\n}")

	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"a":[1,2],"b":{"c":true}}`, dataOf(t, body))
	assert.Equal(t, `{"a":[1,2],"b":{"c":true}}`, store.rows["workspace:w1"],
		"the stored bytes must be the compacted object, not the whitespace the client happened to send")
}

func TestGetUI_ReturnsStoredValueVerbatim(t *testing.T) {
	store := newMemStore()
	store.rows["repo:r1"] = `{"entries":[{"wsId":"w1"}]}`
	srv := newServer(t, store)

	status, body := call(t, srv, http.MethodGet, "repo:r1", "")

	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{"entries":[{"wsId":"w1"}]}`, dataOf(t, body))
}

func TestGetUI_EmptyStoredValue_Returns200EmptyObject(t *testing.T) {
	store := newMemStore()
	store.rows["global"] = ""
	srv := newServer(t, store)

	status, body := call(t, srv, http.MethodGet, "global", "")

	require.Equal(t, http.StatusOK, status)
	assert.JSONEq(t, `{}`, dataOf(t, body))
}

func TestGetUI_StoreError_Returns500(t *testing.T) {
	store := newMemStore()
	store.getErr = errors.New("view.db is closed")
	srv := newServer(t, store)

	status, body := call(t, srv, http.MethodGet, "global", "")

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Contains(t, body, "view.db is closed")
}

func TestPutUI_StoreError_Returns500(t *testing.T) {
	store := newMemStore()
	store.putErr = errors.New("disk full")
	srv := newServer(t, store)

	status, body := call(t, srv, http.MethodPut, "global", `{"a":1}`)

	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Contains(t, body, "disk full")
}

func TestPutUI_NonObjectBody_Returns400(t *testing.T) {
	srv := newServer(t, newMemStore())

	for _, body := range []string{`[]`, `[1]`, `"s"`, `1`, `false`, `null`, `{`, `{}{}`, `oops`} {
		status, _ := call(t, srv, http.MethodPut, "global", body)
		assert.Equalf(t, http.StatusBadRequest, status, "body %q must be rejected", body)
	}
}

func TestPutUI_MissingBody_Returns400(t *testing.T) {
	srv := newServer(t, newMemStore())

	status, _ := call(t, srv, http.MethodPut, "global", "")

	assert.Equal(t, http.StatusBadRequest, status)
}

func TestPutUI_BodyAtAndOverCap(t *testing.T) {
	srv := newServer(t, newMemStore())
	overhead := len(`{"blob":""}`)

	atCap := fmt.Sprintf(`{"blob":%q}`, strings.Repeat("a", settingshandlers.MaxValueBytes-overhead))
	require.Len(t, atCap, settingshandlers.MaxValueBytes)
	status, _ := call(t, srv, http.MethodPut, "global", atCap)
	assert.Equal(t, http.StatusOK, status, "a body of exactly the cap must be accepted")

	overCap := fmt.Sprintf(`{"blob":%q}`, strings.Repeat("a", settingshandlers.MaxValueBytes-overhead+1))
	status, _ = call(t, srv, http.MethodPut, "global", overCap)
	assert.Equal(t, http.StatusRequestEntityTooLarge, status, "one byte over the cap must be refused")
}

func TestScope_AcceptedAndRejectedForms(t *testing.T) {
	srv := newServer(t, newMemStore())

	for _, scope := range []string{
		"global",
		"repo:r1",
		"workspace:w1",
		"workspace:8f14e45f-ceea-467a-9575-2c34ac2a1f9e",
		"repo:a.b_c-d",
		"workspace:" + strings.Repeat("w", 64),
	} {
		status, _ := call(t, srv, http.MethodPut, scope, `{"a":1}`)
		assert.Equalf(t, http.StatusOK, status, "scope %q must be accepted", scope)
	}

	for _, scope := range []string{
		"",
		"Global",
		"globalx",
		"repo",
		"repo:",
		"workspace:",
		"project:p1",
		"workspace:w%2F1",
		"repo:" + strings.Repeat("r", 65),
		"workspace:" + strings.Repeat("w", 200),
	} {
		status, _ := call(t, srv, http.MethodPut, scope, `{"a":1}`)
		assert.Equalf(t, http.StatusBadRequest, status, "scope %q must be rejected on PUT", scope)

		status, _ = call(t, srv, http.MethodGet, scope, "")
		assert.Equalf(t, http.StatusBadRequest, status, "scope %q must be rejected on GET", scope)
	}
}

// TestPutUI_SameScopeWritesAreSerialised is the concurrency guarantee stated as
// an assertion rather than a hope: while a Save for one scope is in flight, no
// second Save for that SAME scope may begin. The stub yields the processor
// repeatedly inside Save, so an unserialised handler would be caught overlapping.
//
// It matters because the underlying GORM Save is UPDATE-then-conditional-INSERT,
// two statements with a window between them: two first-writes to a fresh scope
// can both see the UPDATE match zero rows and both go on to INSERT.
func TestPutUI_SameScopeWritesAreSerialised(t *testing.T) {
	store := newMemStore()
	store.trackScopes = true
	srv := newServer(t, store)

	const writers = 16
	var release sync.WaitGroup
	release.Add(1)
	var joined sync.WaitGroup
	statuses := make([]int, writers)

	for i := 0; i < writers; i++ {
		joined.Add(1)
		go func(idx int) {
			defer joined.Done()
			release.Wait()
			req, err := http.NewRequest(
				http.MethodPut,
				srv.URL+"/v0/settings/ui?scope=workspace:w-hot",
				strings.NewReader(fmt.Sprintf(`{"writer":%d}`, idx)),
			)
			if err != nil {
				return
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			statuses[idx] = resp.StatusCode
		}(i)
	}

	release.Done()
	joined.Wait()

	for i, status := range statuses {
		require.Equalf(t, http.StatusOK, status, "writer %d must have been accepted", i)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&store.maxOverlap),
		"two writes to the same scope must never be in the store at once")
	assert.Regexp(t, `^\{"writer":\d+\}$`, store.rows["workspace:w-hot"],
		"the surviving value must be one writer's object, whole")
}

// TestPutUI_DifferentScopesDoNotContend proves the serialisation is per SCOPE
// and not one global write lock. It matters because the per-workspace layout is
// saved on a debounced loop: a single lock across all scopes would put every
// workspace's save behind every other workspace's, and behind the machine-wide
// preference blob too.
//
// The proof is a rendezvous, not a measurement. Both Saves must be inside the
// store SIMULTANEOUSLY to get past the barrier, so a handler that serialised
// across scopes could never complete this — the first Save would sit waiting for
// a second that the lock is preventing. The test blocks on the barrier itself
// (a real signal); `go test -timeout` is only the backstop for the wedge, never
// the synchronisation.
func TestPutUI_DifferentScopesDoNotContend(t *testing.T) {
	store := newMemStore()
	store.rendezvous = &sync.WaitGroup{}
	store.rendezvous.Add(2)
	srv := newServer(t, store)

	var joined sync.WaitGroup
	statuses := make([]int, 2)
	scopes := []string{"workspace:w-a", "repo:r-b"}

	for i, scope := range scopes {
		joined.Add(1)
		go func(idx int, target string) {
			defer joined.Done()
			req, err := http.NewRequest(
				http.MethodPut,
				srv.URL+"/v0/settings/ui?scope="+target,
				strings.NewReader(`{"a":1}`),
			)
			if err != nil {
				return
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			statuses[idx] = resp.StatusCode
		}(i, scope)
	}

	joined.Wait()

	for i, status := range statuses {
		assert.Equalf(t, http.StatusOK, status, "write to %s must have completed", scopes[i])
	}
	assert.Len(t, store.rows, 2, "both scopes must have been written")
}
