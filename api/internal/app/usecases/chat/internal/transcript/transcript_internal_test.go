package transcript

import (
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// One chat's publication and reads never wait on another chat: while a read of
// chat a waits inside the projector for a's lock, chat b's rows are applied and read.
func TestProjector_AChatsApplyNeverWaitsOnAnotherChat(t *testing.T) {
	p := New(nil)
	opened, err := ledger.NewRow(ledger.KindTurnStarted, "T1", "", ledger.TurnStarted{RunnerID: "run-1"})
	require.NoError(t, err)
	opened.Seq = 1
	for _, chatID := range []string{"a", "b"} {
		p.install(chatID, []ledger.Row{opened})
	}
	a, ok := p.entry("a")
	require.True(t, ok)
	a.mu.Lock()
	release := sync.OnceFunc(a.mu.Unlock)
	defer release()
	readA := make(chan error, 1)
	go func() {
		_, err := p.Messages(t.Context(), "a", 0, 0, 10)
		readA <- err
	}()
	require.Eventually(t, func() bool { return waitsIn("(*Projector).cached") },
		5*time.Second, time.Millisecond, "precondition: the read of a waits for a's lock")

	said, err := ledger.NewRow(ledger.KindAssistantText, "T1", "", ledger.AssistantText{
		MessageID: "m1", Index: ledger.IndexComplete, Text: "said",
	})
	require.NoError(t, err)
	said.Seq = 2
	readB := make(chan []domain.LedgerMessage, 1)
	go func() {
		p.Apply("b", []ledger.Row{said})
		page, err := p.Messages(t.Context(), "b", 0, 0, 10)
		if err == nil {
			readB <- page.Items
		}
		close(readB)
	}()
	select {
	case items := <-readB:
		require.Len(t, items, 1)
		require.Equal(t, "said", items[0].Text)
	case <-time.After(5 * time.Second):
		require.Fail(t, "chat b waited on chat a's read")
	}

	release()
	require.NoError(t, <-readA)
}

// waitsIn reports whether a goroutine is parked on a lock inside fn.
func waitsIn(fn string) bool {
	buf := make([]byte, 1<<20)
	for _, g := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(g, fn) && strings.Contains(g, "sync.(*Mutex).Lock") {
			return true
		}
	}
	return false
}
