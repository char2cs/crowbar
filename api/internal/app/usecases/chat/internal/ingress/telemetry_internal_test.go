package ingress

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestRecorder_TelemetryIsHeldAndPushed(t *testing.T) {
	f := newRecorderFixture(t, "claude")

	require.NoError(t, f.r.RecordTelemetry(t.Context(), f.runner, f.agent, []byte(`{}`), engineagents.ChannelHooks), "an empty report is no report")
	_, ok := f.r.Telemetry(testChat)
	assert.False(t, ok)

	require.NoError(t, f.r.RecordTelemetry(t.Context(), f.runner, f.agent,
		[]byte(`{"context_window":{"context_window_size":200000,"used_percentage":19},"model":{"id":"m","display_name":"M"}}`), engineagents.ChannelHooks))
	_, ok = f.r.Telemetry(testChat)
	assert.True(t, ok)
	assert.Equal(t, 1, f.feed.telemetry)

	err := f.r.RecordTelemetry(t.Context(), f.runner, f.agent, []byte(`not json`), engineagents.ChannelHooks)
	require.ErrorIs(t, err, problem.ErrUnprocessable)
	gone := f.runner
	gone.CurrentChatID = ""
	require.ErrorIs(t, f.r.RecordTelemetry(t.Context(), gone, f.agent, []byte(`{}`), engineagents.ChannelHooks), problem.ErrNotFound)
}

func TestRecorder_TelemetryReadsTheChannelItArrivedOn(t *testing.T) {
	f := newRecorderFixture(t, "hermes")
	raw := []byte(`{"session_id":"s1","type":"session.usage","payload":{"usage":{
	  "context_max":1000000,"context_percent":2,"context_used":19510,"model":"m"}}}`)

	err := f.r.RecordTelemetry(t.Context(), f.runner, f.agent, raw, engineagents.ChannelHooks)
	require.ErrorIs(t, err, problem.ErrUnprocessable, "the hooks channel has no telemetry block here")

	require.NoError(t, f.r.RecordTelemetry(t.Context(), f.runner, f.agent, raw, engineagents.ChannelAPI))
	report, ok := f.r.Telemetry(testChat)
	require.True(t, ok)
	assert.Equal(t, 19510, *report.Context.UsedTokens)
}
