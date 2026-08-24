package telemetry_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
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
	s.Set("chat-1", engineagents.Telemetry{Source: "statusline", Model: &engineagents.ModelIdentity{ID: "opus"}})
	s.Set("chat-1", engineagents.Telemetry{Source: "jsonrpc"})

	report, ok := s.Get("chat-1")

	require.True(t, ok)
	assert.Equal(t, "jsonrpc", report.Source)
	assert.Nil(t, report.Model, "a provider restates its whole telemetry; a stale field must not survive")
}

func TestForget_DropsOnlyThatChat(t *testing.T) {
	t.Parallel()

	s := telemetry.New()
	s.Set("chat-1", engineagents.Telemetry{Source: "a"})
	s.Set("chat-2", engineagents.Telemetry{Source: "b"})

	s.Forget("chat-1")

	_, ok := s.Get("chat-1")
	assert.False(t, ok, "a purged chat must not leave a number behind")
	_, ok = s.Get("chat-2")
	assert.True(t, ok)
}

func TestForget_OfAnUnknownChatIsSilent(t *testing.T) {
	t.Parallel()

	telemetry.New().Forget("never-seen")
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
		go func() { defer wg.Done(); <-start; s.Set("chat-1", engineagents.Telemetry{Source: strconv.Itoa(i)}) }()
		go func() { defer wg.Done(); <-start; s.Get("chat-1") }()
	}
	close(start)
	wg.Wait()

	_, ok := s.Get("chat-1")
	assert.True(t, ok)
}
