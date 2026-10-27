package internal_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// Until Begin hands over the session (here, a home it refuses), the gate says
// Crowbar is starting: health has no detail, and the API refuses in the same words.
func TestReport_IsStartingUntilTheSessionIsPublished(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")
	sock := testutil.ShortSocket(t)
	client := testutil.UnixClient(t, sock)
	var health, refusal answer
	askBeforePublish := slog.New(testutil.OnLog("migrate: halted", func(slog.Record) {
		health = call(t, client, http.MethodGet, "/v0/health", nil)
		refusal = call(t, client, http.MethodGet, "/v0/projects", nil)
	}))

	c, err := internal.New(t.Context(), "unix://"+sock, nil,
		internal.WithHomeDir(h.Root()), internal.WithMigrateLog(askBeforePublish))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	assert.Equal(t, http.StatusOK, health.status)
	assert.Equal(t, "migrating", health.Data.Status, health.raw)
	assert.Equal(t, "null", string(health.Data.Detail), health.raw)
	assert.Equal(t, http.StatusServiceUnavailable, refusal.status)
	assert.Equal(t, "daemon_migrating", refusal.Code)
	assert.Equal(t, "Crowbar is starting", refusal.Error)
}

// progress is the health detail of a daemon still migrating.
type progress struct {
	Phase string     `json:"phase"`
	Step  migrate.ID `json:"step"`
	Index int        `json:"index"`
	Total int        `json:"total"`
}

// While a step runs, health names it, from the first pre-open step on, and
// every other request is refused with the same words.
func TestReport_NamesTheStepThatIsRunning(t *testing.T) {
	h := testutil.FixtureHome(t)
	sock := testutil.ShortSocket(t)
	client := testutil.UnixClient(t, sock)
	var healths, refusals []answer
	askAtEachStep := slog.New(testutil.OnLog("migrate: step started", func(slog.Record) {
		healths = append(healths, call(t, client, http.MethodGet, "/v0/health", nil))
		refusals = append(refusals, call(t, client, http.MethodGet, "/v0/projects", nil))
	}))

	c, err := internal.New(t.Context(), "unix://"+sock, nil,
		internal.WithHomeDir(h.Root()), internal.WithMigrateLog(askAtEachStep))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	require.Len(t, healths, 6)
	for _, health := range healths {
		assert.Equal(t, http.StatusOK, health.status)
		assert.Equal(t, "migrating", health.Data.Status, health.raw)
	}
	assert.Equal(t, progress{Phase: "pre-open", Step: migrate.ID{Number: 2, Name: "workspace-paths-table"}, Index: 1, Total: 6},
		detail[progress](t, healths[0]))
	assert.Equal(t, progress{Phase: "pre-open", Step: migrate.ID{Number: 6, Name: "chat-ledger"}, Index: 2, Total: 6},
		detail[progress](t, healths[1]))
	assert.Equal(t, progress{Phase: "post-open", Step: migrate.ID{Number: 1, Name: "project-homes"}, Index: 3, Total: 6},
		detail[progress](t, healths[2]))
	assert.Equal(t, http.StatusServiceUnavailable, refusals[0].status)
	assert.Equal(t, "daemon_migrating", refusals[0].Code)
	assert.Equal(t, "Crowbar is upgrading its data (step 1 of 6: workspace-paths-table)", refusals[0].Error)
	assert.Equal(t, "Crowbar is upgrading its data (step 2 of 6: chat-ledger)", refusals[1].Error)
	assert.Equal(t, "Crowbar is upgrading its data (step 6 of 6: hook-deliveries)", refusals[5].Error)
}
