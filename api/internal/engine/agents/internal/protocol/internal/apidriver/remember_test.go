package apidriver_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// capturedStep is the step a delivered frame carries from what an earlier frame remembered.
func capturedStep(t *testing.T, raw []byte) (string, bool) {
	t.Helper()
	var params struct {
		Captured map[string]string `json:"captured"`
	}
	require.NoError(t, json.Unmarshal(raw, &params))
	step, ok := params.Captured["step"]
	return step, ok
}

func TestStart_RememberedFrameValuesRideTheFramesAfterThem(t *testing.T) {
	itemStarted := fixtureLine(t, "item_started.json")
	stdio := fakeCodexServer(t, func(conn *testutil.LinePeer) {
		for _, frame := range []map[string]any{
			{"method": "step/begin", "params": map[string]any{"kind": "step", "iteration": 1}},
			{"method": "step/begin", "params": map[string]any{"kind": "other", "iteration": 9}},
			{"method": "step/begin", "params": map[string]any{"kind": "step", "iteration": 2}},
			{"method": "step/begin", "params": map[string]any{"kind": "step", "iteration": nil}},
		} {
			require.NoError(t, conn.WriteJSON(frame))
			require.NoError(t, conn.Write(itemStarted))
		}
		_, _ = conn.Read()
	})
	d := loadCodexAPIDescriptor(t)
	d.Runtime.API.Remember = []spec.FrameCapture{{
		In: spec.WireRef{"step/begin"}, When: spec.WhenMap{"kind": {"step"}}, Capture: map[string]string{"step": "iteration"},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	drv, err := apidriver.Start(ctx, d, stdio, nil)
	require.NoError(t, err)
	defer func() { _ = drv.Close() }()

	var got []string
	for range 4 {
		select {
		case ev := <-drv.Events():
			require.Equal(t, "user_prompt", ev.Canonical)
			step, _ := capturedStep(t, ev.Raw)
			got = append(got, step)
		case <-time.After(5 * time.Second):
			require.FailNow(t, "a frame never arrived")
		}
	}

	assert.Equal(t, []string{"1", "1", "2", "2"}, got,
		"a frame its when: excludes captures nothing, and a null value keeps the last one")
}

func TestStart_NothingRememberedAddsNothingToAFrame(t *testing.T) {
	stdio := fakeCodexServer(t, func(conn *testutil.LinePeer) {
		require.NoError(t, conn.Write(fixtureLine(t, "turn_completed.json")))
		_, _ = conn.Read()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	drv, err := apidriver.Start(ctx, loadCodexAPIDescriptor(t), stdio, nil)
	require.NoError(t, err)
	defer func() { _ = drv.Close() }()

	ev := <-drv.Events()

	_, ok := capturedStep(t, ev.Raw)
	assert.False(t, ok)
}
