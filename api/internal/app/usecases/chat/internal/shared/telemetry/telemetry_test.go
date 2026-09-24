package telemetry_test

import (
	"context"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// A chat nobody has reported for is UNKNOWN, not zero. The UI shows a context
// gauge off this, and "0% used" is a claim; "no report yet" is the truth.
func TestGet_UnreportedChatIsUnknownNotZero(t *testing.T) {
	t.Parallel()

	s := telemetry.New()

	report, ok := s.Get("chat-1")

	assert.False(t, ok)
	assert.Equal(t, engineagents.Telemetry{}, report)
}

func TestSet_ReplacesRatherThanMerges(t *testing.T) {
	t.Parallel()

	s := telemetry.New()
	s.Set(context.Background(), "chat-1", engineagents.Telemetry{Source: "statusline", Model: &engineagents.ModelIdentity{ID: "opus"}})
	s.Set(context.Background(), "chat-1", engineagents.Telemetry{Source: "jsonrpc"})

	report, ok := s.Get("chat-1")

	require.True(t, ok)
	assert.Equal(t, "jsonrpc", report.Source)
	assert.Nil(t, report.Model, "a provider restates its whole telemetry; a stale field must not survive")
}

func TestForget_DropsOnlyThatChat(t *testing.T) {
	t.Parallel()

	s := telemetry.New()
	s.Set(context.Background(), "chat-1", engineagents.Telemetry{Source: "a"})
	s.Set(context.Background(), "chat-2", engineagents.Telemetry{Source: "b"})

	s.Forget(context.Background(), "chat-1")

	_, ok := s.Get("chat-1")
	assert.False(t, ok, "a purged chat must not leave a number behind")
	_, ok = s.Get("chat-2")
	assert.True(t, ok)
}

func TestForget_OfAnUnknownChatIsSilent(t *testing.T) {
	t.Parallel()

	telemetry.New().Forget(context.Background(), "never-seen")
}

// Reports arrive on hook goroutines while the chats panel polls. Under -race a
// store that is not actually guarded fails here.
func TestStore_IsSafeUnderConcurrentReportsAndReads(t *testing.T) {
	t.Parallel()

	s := telemetry.New()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 32 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			s.Set(context.Background(), "chat-1", engineagents.Telemetry{Source: strconv.Itoa(i)})
		}()
		go func() { defer wg.Done(); <-start; s.Get("chat-1") }()
	}
	close(start)
	wg.Wait()

	_, ok := s.Get("chat-1")
	assert.True(t, ok)
}

// newPersistedStore opens a real sqlite-backed AgentChatTelemetry store at a
// temp path — the same generic store production wires through app/gorm.go —
// so these tests pin the actual persistence contract, not a fake of it.
func newPersistedStore(t *testing.T) (dbPath string) {
	t.Helper()
	return filepath.Join(t.TempDir(), "telemetry.db")
}

func openStore(t *testing.T, path string) telemetry.Persistence {
	t.Helper()
	store, err := storesqlite.New[domain.AgentChatTelemetry, string](path)
	require.NoError(t, err)
	return store
}

// A durable Store must recover a chat's last report from a FRESH construction
// over the same backing store — this is what a daemon restart does: the
// process-local map is gone, but NewDurable seeds a new one from disk. This is
// the exact reproduction of the live bug: before this existed, telemetry.New()
// was always empty on every construction, restart or not.
func TestNewDurable_RecoversReportsWrittenByAPriorStore(t *testing.T) {
	t.Parallel()

	path := newPersistedStore(t)
	persist := openStore(t, path)

	before := telemetry.NewDurable(context.Background(), persist)
	before.Set(context.Background(), "chat-1", engineagents.Telemetry{
		Source:  "statusline",
		Context: &engineagents.ContextUsage{UsedPercent: floatPtr(42)},
	})

	// A brand-new Store, as a restarted daemon would construct, over the SAME
	// persistence — the in-memory map is gone, but the disk row is not.
	after := telemetry.NewDurable(context.Background(), persist)
	report, ok := after.Get("chat-1")

	require.True(t, ok, "a durable store must recover a chat's last report across a fresh construction")
	assert.Equal(t, "statusline", report.Source)
	require.NotNil(t, report.Context)
	require.NotNil(t, report.Context.UsedPercent)
	assert.InDelta(t, 42, *report.Context.UsedPercent, 0.001)
}

// Forget must purge the durable row too, not just the in-memory copy — a
// deleted chat must not come back on the next restart.
func TestNewDurable_ForgetPurgesTheDurableRow(t *testing.T) {
	t.Parallel()

	path := newPersistedStore(t)
	persist := openStore(t, path)

	s := telemetry.NewDurable(context.Background(), persist)
	s.Set(context.Background(), "chat-1", engineagents.Telemetry{Source: "a"})
	s.Forget(context.Background(), "chat-1")

	reopened := telemetry.NewDurable(context.Background(), persist)
	_, ok := reopened.Get("chat-1")
	assert.False(t, ok, "a forgotten chat must not reappear after a restart")
}

// NewDurable(ctx, nil) is the same contract as New() — the seam production and
// every non-durable test share, so a caller with no persistence configured
// still gets a working, process-local-only store rather than a panic.
func TestNewDurable_WithNilPersistenceBehavesLikePlainNew(t *testing.T) {
	t.Parallel()

	s := telemetry.NewDurable(context.Background(), nil)
	s.Set(context.Background(), "chat-1", engineagents.Telemetry{Source: "a"})

	report, ok := s.Get("chat-1")
	require.True(t, ok)
	assert.Equal(t, "a", report.Source)
}

func floatPtr(v float64) *float64 { return &v }
