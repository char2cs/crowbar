//go:build !windows

package session

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

func pinFrameClock(s *Session) time.Time {
	pinned := time.Now()
	s.SetNowForTest(func() time.Time { return pinned })
	return pinned
}

func parkTrailingTimer(s *Session, pinned time.Time) {
	s.mu.Lock()
	s.lastEmitAt = pinned.Add(time.Hour)
	s.mu.Unlock()
}

func coldFrameClock(s *Session) {
	s.mu.Lock()
	s.lastEmitAt = time.Time{}
	s.mu.Unlock()
}

func collectUntil(
	t *testing.T,
	ch <-chan OutputFrame,
	wants ...string,
) string {
	t.Helper()
	var data string
	for {
		f, ok := <-ch
		if !ok {
			t.Fatalf("client channel closed before any of %q arrived; saw: %q", wants, data)
		}
		data += string(f.Data)
		for _, w := range wants {
			if strings.Contains(data, w) {
				return data
			}
		}
	}
}

func quiesce(t *testing.T, s *Session, ch <-chan OutputFrame) {
	t.Helper()
	waitPrompt(t, s)
	drainFrames(ch)
}

func TestModelDriven_OutputIsModelDerived(t *testing.T) {
	s, err := New(t.Context(), "sid-md", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	require.NoError(t, s.Write([]byte("echo MD-MARKER-42\n")))
	data := collectUntil(t, ch, "MD-MARKER-42")
	assert.Contains(t, data, "\x1b[", "model-driven output is synthesized ANSI")
}

func TestModelDriven_AttachFlushesPendingDeltaBeforeRebasing(t *testing.T) {
	s := newBareSession("sid-md-flush", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	pinned := pinFrameClock(s)

	ch1, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch1)
	_, ok := waitFrame(t, ch1)
	require.True(t, ok, "attach must deliver an initial snapshot")

	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch1)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("PENDING-DELTA-XYZ"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must arm the trailing timer (pending delta)")

	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)

	f1, ok := waitFrame(t, ch1)
	require.True(t, ok, "Attach must flush the pending delta to the existing client")
	assert.False(t, f1.Snapshot, "the flushed pending delta is an incremental diff, not a snapshot")
	assert.Contains(t, string(f1.Data), "PENDING-DELTA-XYZ",
		"the flushed delta frame must carry the pre-attach write's text, not silently drop it")

	f2, ok := waitFrame(t, ch2)
	require.True(t, ok, "second attach must deliver a snapshot")
	assert.True(t, f2.Snapshot)
	assert.Contains(t, string(f2.Data), "PENDING-DELTA-XYZ",
		"the new client's attach snapshot must already include the pre-attach write")
}

func TestModelDriven_BurstCoalescesFrames(t *testing.T) {
	s := newBareSession("sid-md-burst", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 2000)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	pinned := pinFrameClock(s)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	parkTrailingTimer(s, pinned)

	const burstChunks = 50
	for i := 0; i < burstChunks; i++ {
		s.pumpStep([]byte(fmt.Sprintf("line%d\n", i)))
	}

	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "the burst must have coalesced onto a trailing timer")

	require.Empty(t, ch, "a burst inside one interval must emit NO per-chunk frames; all 50 coalesce")

	s.mu.Lock()
	s.flushPendingEmitLocked()
	s.mu.Unlock()

	f, ok := waitFrame(t, ch)
	require.True(t, ok, "the pending trailing delta must flush as a frame")
	require.Contains(t, string(f.Data), "line0", "the coalesced frame carries the whole burst")
	require.Contains(t, string(f.Data), fmt.Sprintf("line%d", burstChunks-1),
		"the coalesced frame carries the whole burst, through its last chunk")
	require.Empty(t, ch, "the coalesced burst yields exactly ONE frame, never a second")
}

func TestModelDriven_BurstOverLivePTYDeliversAllOutput(t *testing.T) {
	s, err := New(t.Context(), "sid-md-burst-live", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 2000)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	quiesce(t, s, ch)

	require.NoError(t, s.Write([]byte("seq 1 200; printf '%s%s\\n' SEQ END\n")))
	data := collectUntil(t, ch, "SEQEND")
	assert.Contains(t, data, "200", "burst output must fully arrive regardless of coalescing frame count")
}

func TestModelDriven_TrailingTimerFlushesFinalState(t *testing.T) {
	s, err := New(t.Context(), "sid-md-trail", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)
	quiesce(t, s, ch)

	require.NoError(t, s.Write([]byte("echo TRAILING-EDGE-OK\n")))
	collectUntil(t, ch, "TRAILING-EDGE-OK")
}

func TestModelDriven_AttachDisarmsStaleTrailingTimer(t *testing.T) {
	s := newBareSession("sid-md-attach-timer", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	pinned := pinFrameClock(s)

	ch1, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch1)
	_, ok := waitFrame(t, ch1)
	require.True(t, ok, "attach must deliver an initial snapshot")

	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch1)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must coalesce onto the trailing timer")

	ch2, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch2)

	s.mu.Lock()
	stillArmed := s.emitTimer != nil
	s.mu.Unlock()
	assert.False(t, stillArmed, "Attach must disarm the stale trailing timer")

	_, ok = waitFrame(t, ch1)
	require.True(t, ok, "Attach must flush the pending delta to the existing client")

	f2, ok := waitFrame(t, ch2)
	require.True(t, ok, "second attach must deliver a snapshot")
	assert.True(t, f2.Snapshot)

	coldFrameClock(s)
	s.pumpStep([]byte("POST-ATTACH-BARRIER"))

	next, ok := waitFrame(t, ch1)
	require.True(t, ok, "the post-attach chunk must reach the existing client")
	assert.Contains(t, string(next.Data), "POST-ATTACH-BARRIER",
		"the next frame on ch1 must be the post-attach chunk: a stale trailing timer must not have "+
			"fired a duplicate delta ahead of it")
}

func TestModelDriven_TeardownStopsTrailingTimer(t *testing.T) {
	s := newBareSession("sid-md-teardown-timer", "/bin/sh", t.TempDir(), "")
	m, ser := newModel(80, 24, 200)
	s.model = m
	s.serializer = ser
	s.emitter = model.NewDiffEmitter()

	pinned := pinFrameClock(s)

	ch, err := s.Attach()
	require.NoError(t, err)
	_, ok := waitFrame(t, ch)
	require.True(t, ok, "attach must deliver an initial snapshot")

	s.pumpStep([]byte("echo A\n"))
	_, ok = waitFrame(t, ch)
	require.True(t, ok, "first chunk must emit immediately (cold clock)")

	parkTrailingTimer(s, pinned)
	s.pumpStep([]byte("echo B\n"))
	s.mu.Lock()
	armed := s.emitTimer != nil
	s.mu.Unlock()
	require.True(t, armed, "test setup: the second chunk must coalesce onto the trailing timer")

	require.NotPanics(t, func() { s.Kill() })

	<-s.Done()

	s.mu.Lock()
	stillArmed := s.emitTimer != nil
	s.mu.Unlock()
	assert.False(t, stillArmed, "teardown must stop the armed trailing timer")

	frames := 0
	for f := range ch {
		frames++
		t.Logf("unexpected frame after teardown: %+v", f)
	}
	assert.Equal(t, 0, frames, "no frame may arrive after teardown (trailing timer must have been stopped)")
}

func TestModelDriven_CPRQueryAnsweredToPTY(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable; CPR read vehicle requires bash (read -s/-d/-t)")
	}
	s, err := New(t.Context(), "sid-md-cpr", bash, t.TempDir(), "", os.Environ(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	script := "printf '\\x1b[6n'; IFS= read -rs -t 3 -d R REPLY; " +
		"if [ -n \"$REPLY\" ]; then printf 'CPR'; printf -- '-ANSWERED\\n'; else printf 'NO'; printf 'ANSWER\\n'; fi\n"
	require.NoError(t, s.Write([]byte(script)))

	data := collectUntil(t, ch, "CPR-ANSWERED", "NOANSWER")
	assert.Contains(t, data, "CPR-ANSWERED")
	assert.NotContains(t, data, "NOANSWER")
}

func TestModelDriven_ModelResetKeepsOneAnswerer(t *testing.T) {
	s, err := New(t.Context(), "sid-md-sink-reset", "/bin/sh", t.TempDir(), "", testEnv(), 80, 24, 200)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	ch, err := s.Attach()
	require.NoError(t, err)
	defer s.Detach(ch)

	s.mu.Lock()
	s.resetModelLocked()
	installed := model.ResponseSinkInstalledForTest(s.model)
	s.mu.Unlock()
	assert.True(t, installed, "the rebuilt model must answer device queries")
	_, panics := s.Health()
	assert.Equal(t, 1, panics, "the recovered panic is counted")

	require.NoError(t, s.Write([]byte("echo AFTER-RESET\n")))
	data := collectUntil(t, ch, "AFTER-RESET")
	assert.Contains(t, data, "AFTER-RESET", "the rebuilt model keeps serving model-derived output")
}
