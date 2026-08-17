package telemetry_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/telemetry"
)

var at = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

func withCallback(fields map[string]string, windows ...spec.TelemetryRateLimitMap) *spec.Descriptor {
	return &spec.Descriptor{
		ID: "probe",
		Telemetry: &spec.TelemetrySpec{Callback: &spec.TelemetryCallbackSpec{
			Format: "json", Fields: fields, RateLimits: windows,
		}},
	}
}

// The shape claude actually sends, measured 2026-08-17.
const claudeStatusLine = `{
  "context_window":{"context_window_size":200000,"used_percentage":19,
                    "remaining_percentage":81,"total_input_tokens":37117},
  "cost":{"total_cost_usd":0.0649,"total_api_duration_ms":8123},
  "rate_limits":{"five_hour":{"used_percentage":1,"resets_at":"2026-08-17T15:00:00Z"},
                 "seven_day":{"used_percentage":47,"resets_at":"2026-08-24T00:00:00Z"}},
  "model":{"id":"claude-haiku-4-5-20251001","display_name":"Haiku 4.5"}}`

func claudeDescriptor() *spec.Descriptor {
	return withCallback(map[string]string{
		spec.FactContextCapacityTokens: "context_window.context_window_size",
		spec.FactContextUsedTokens:     "context_window.total_input_tokens",
		spec.FactContextUsedPercent:    "context_window.used_percentage",
		spec.FactContextRemainPercent:  "context_window.remaining_percentage",
		spec.FactCostTotalUSD:          "cost.total_cost_usd",
		spec.FactCostAPIDurationMS:     "cost.total_api_duration_ms",
		spec.FactModelID:               "model.id",
		spec.FactModelDisplayName:      "model.display_name",
	},
		spec.TelemetryRateLimitMap{
			ID: "five_hour", Label: "5-hour",
			UsedPercent: "rate_limits.five_hour.used_percentage", ResetsAt: "rate_limits.five_hour.resets_at",
		},
		spec.TelemetryRateLimitMap{
			ID: "seven_day", Label: "7-day",
			UsedPercent: "rate_limits.seven_day.used_percentage", ResetsAt: "rate_limits.seven_day.resets_at",
		},
	)
}

func TestParseCallback_MapsEveryReportedFact(t *testing.T) {
	got, err := telemetry.ParseCallback(claudeDescriptor(), []byte(claudeStatusLine), at)

	require.NoError(t, err)
	assert.Equal(t, at, got.ObservedAt)
	assert.Equal(t, models.TelemetrySourceCallback, got.Source)

	require.NotNil(t, got.Context)
	assert.Equal(t, 200000, *got.Context.CapacityTokens)
	assert.Equal(t, 37117, *got.Context.UsedTokens)
	assert.InDelta(t, 19, *got.Context.UsedPercent, 0.001)
	assert.InDelta(t, 81, *got.Context.RemainingPercent, 0.001)

	require.NotNil(t, got.Cost)
	assert.InDelta(t, 0.0649, *got.Cost.TotalUSD, 0.00001)
	assert.Equal(t, 8123, *got.Cost.APIDurationMS)

	require.NotNil(t, got.Model)
	assert.Equal(t, "claude-haiku-4-5-20251001", got.Model.ID)
	assert.Equal(t, "Haiku 4.5", got.Model.DisplayName)

	require.Len(t, got.RateLimits, 2)
	assert.Equal(t, "five_hour", got.RateLimits[0].ID)
	assert.Equal(t, "5-hour", got.RateLimits[0].Label)
	assert.InDelta(t, 1, *got.RateLimits[0].UsedPercent, 0.001)
	assert.Equal(t, time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC), got.RateLimits[0].ResetsAt.UTC())
}

// Usage is null until the first turn completes, so an early report is genuinely
// empty rather than a reset. It must not render as a gauge at zero.
func TestParseCallback_AFreshSessionReportsNothingRatherThanZero(t *testing.T) {
	got, err := telemetry.ParseCallback(claudeDescriptor(),
		[]byte(`{"context_window":null,"cost":null,"model":null,"rate_limits":null}`), at)

	require.NoError(t, err)
	assert.True(t, got.Empty())
	assert.Nil(t, got.Context)
	assert.Nil(t, got.Cost)
	assert.Nil(t, got.Model)
	assert.Nil(t, got.RateLimits)
}

func TestParseCallback_AGroupWithOneReportedFactIsPresent(t *testing.T) {
	d := withCallback(map[string]string{spec.FactCostTotalUSD: "cost.total_cost_usd"})

	got, err := telemetry.ParseCallback(d, []byte(`{"cost":{"total_cost_usd":1.5}}`), at)

	require.NoError(t, err)
	require.NotNil(t, got.Cost)
	assert.InDelta(t, 1.5, *got.Cost.TotalUSD, 0.0001)
	assert.Nil(t, got.Cost.APIDurationMS, "an unreported fact stays absent, not zero")
	assert.Nil(t, got.Context)
}

// A percentage is computed only when BOTH operands were reported. Anything beyond
// that would be an estimate wearing a gauge's clothes.
func TestParseCallback_DerivesOnlyWhatFollowsArithmeticallyFromReportedFacts(t *testing.T) {
	d := withCallback(map[string]string{
		spec.FactContextCapacityTokens: "cap",
		spec.FactContextUsedTokens:     "used",
	})

	got, err := telemetry.ParseCallback(d, []byte(`{"cap":1000,"used":250}`), at)

	require.NoError(t, err)
	require.NotNil(t, got.Context)
	require.NotNil(t, got.Context.UsedPercent)
	assert.InDelta(t, 25, *got.Context.UsedPercent, 0.0001)
	require.NotNil(t, got.Context.RemainingPercent)
	assert.InDelta(t, 75, *got.Context.RemainingPercent, 0.0001)
}

func TestParseCallback_DoesNotDivideByAnUnknownOrZeroCapacity(t *testing.T) {
	d := withCallback(map[string]string{
		spec.FactContextCapacityTokens: "cap",
		spec.FactContextUsedTokens:     "used",
	})

	zero, err := telemetry.ParseCallback(d, []byte(`{"cap":0,"used":250}`), at)
	require.NoError(t, err)
	require.NotNil(t, zero.Context)
	assert.Nil(t, zero.Context.UsedPercent)

	unknown, err := telemetry.ParseCallback(d, []byte(`{"used":250}`), at)
	require.NoError(t, err)
	require.NotNil(t, unknown.Context)
	assert.Nil(t, unknown.Context.UsedPercent)
}

func TestParseCallback_AReportedPercentageIsNeverOverwritten(t *testing.T) {
	d := withCallback(map[string]string{
		spec.FactContextCapacityTokens: "cap",
		spec.FactContextUsedTokens:     "used",
		spec.FactContextUsedPercent:    "pct",
	})

	got, err := telemetry.ParseCallback(d, []byte(`{"cap":1000,"used":250,"pct":31}`), at)

	require.NoError(t, err)
	assert.InDelta(t, 31, *got.Context.UsedPercent, 0.0001,
		"what the provider reported wins over what Crowbar could compute")
}

// A window the provider did not report this time is omitted rather than carried
// as an empty row the UI renders as a gauge at zero.
func TestParseCallback_OmitsAnUnreportedRateLimitWindow(t *testing.T) {
	d := withCallback(
		map[string]string{spec.FactModelID: "model.id"},
		spec.TelemetryRateLimitMap{ID: "five_hour", UsedPercent: "rl.five.pct"},
		spec.TelemetryRateLimitMap{ID: "weekly", UsedPercent: "rl.weekly.pct"},
	)

	got, err := telemetry.ParseCallback(d, []byte(`{"model":{"id":"m"},"rl":{"five":{"pct":3}}}`), at)

	require.NoError(t, err)
	require.Len(t, got.RateLimits, 1)
	assert.Equal(t, "five_hour", got.RateLimits[0].ID)
}

func TestParseCallback_AnUnparseableResetTimeIsAbsentRatherThanZero(t *testing.T) {
	d := withCallback(nil, spec.TelemetryRateLimitMap{
		ID: "w", UsedPercent: "pct", ResetsAt: "resets",
	})

	got, err := telemetry.ParseCallback(d, []byte(`{"pct":5,"resets":"soon"}`), at)

	require.NoError(t, err)
	require.Len(t, got.RateLimits, 1)
	assert.Nil(t, got.RateLimits[0].ResetsAt,
		"a zero reset time renders as 'resets in -2000 years', which is worse than nothing")
}

func TestParseCallback_UnsupportedWhenTheProviderDeclaresNoTelemetry(t *testing.T) {
	_, err := telemetry.ParseCallback(&spec.Descriptor{ID: "codex"}, []byte(`{}`), at)
	assert.ErrorIs(t, err, telemetry.ErrUnsupported)

	_, err = telemetry.ParseCallback(nil, []byte(`{}`), at)
	assert.ErrorIs(t, err, telemetry.ErrUnsupported)
}

func TestParseCallback_RejectsAnUnsupportedFormatAndMalformedBytes(t *testing.T) {
	d := withCallback(map[string]string{spec.FactModelID: "m"})
	d.Telemetry.Callback.Format = "toml"
	_, err := telemetry.ParseCallback(d, []byte(`{}`), at)
	assert.ErrorIs(t, err, telemetry.ErrUnsupportedFormat)

	_, err = telemetry.ParseCallback(withCallback(map[string]string{spec.FactModelID: "m"}),
		[]byte(`{not json`), at)
	assert.Error(t, err)
}

func TestParseCallback_AnEmptyPayloadIsEmptyRatherThanAnError(t *testing.T) {
	got, err := telemetry.ParseCallback(withCallback(map[string]string{spec.FactModelID: "m"}), nil, at)

	require.NoError(t, err)
	assert.True(t, got.Empty())
}

func TestParseProbe_MapsTheSameFactsFromACrowbarInvokedCommand(t *testing.T) {
	d := &spec.Descriptor{ID: "codex", Telemetry: &spec.TelemetrySpec{
		Probe: &spec.TelemetryProbeSpec{
			Format:  "json",
			Command: []string{"debug", "models"},
			Fields:  map[string]string{spec.FactContextCapacityTokens: "context_window"},
		},
	}}

	got, err := telemetry.ParseProbe(d, []byte(`{"context_window":400000}`), at)

	require.NoError(t, err)
	assert.Equal(t, models.TelemetrySourceProbe, got.Source)
	require.NotNil(t, got.Context)
	assert.Equal(t, 400000, *got.Context.CapacityTokens)
	assert.Empty(t, got.RateLimits, "a probe declares no windows")
}

func TestParseProbe_UnsupportedWhenNoProbeIsDeclared(t *testing.T) {
	_, err := telemetry.ParseProbe(withCallback(map[string]string{spec.FactModelID: "m"}), []byte(`{}`), at)
	assert.ErrorIs(t, err, telemetry.ErrUnsupported)
}

func TestParseProbe_RejectsAnUnsupportedFormat(t *testing.T) {
	d := &spec.Descriptor{ID: "codex", Telemetry: &spec.TelemetrySpec{
		Probe: &spec.TelemetryProbeSpec{Format: "toml", Fields: map[string]string{spec.FactModelID: "m"}},
	}}

	_, err := telemetry.ParseProbe(d, []byte(`{}`), at)

	assert.ErrorIs(t, err, telemetry.ErrUnsupportedFormat)
}
