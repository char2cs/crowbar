package spec

// Telemetry fact paths. These are Crowbar's vocabulary, not any provider's: a
// descriptor maps its own payload shape onto these keys, so a Claude
// `context_window.used_percentage` and a future Codex `session.context.pct_used`
// both land on FactContextUsedPercent with no code between them.
//
// The capability is named `telemetry` and NOT after the mechanism any one
// provider uses to deliver it. The moment the key is called `status_line`, the
// abstraction has failed.
const (
	FactContextCapacityTokens = "context.capacity_tokens"
	FactContextUsedTokens     = "context.used_tokens"
	FactContextUsedPercent    = "context.used_percent"
	FactContextRemainPercent  = "context.remaining_percent"

	FactCostTotalUSD      = "cost.total_usd"
	FactCostAPIDurationMS = "cost.api_duration_ms"

	FactModelID          = "model.id"
	FactModelDisplayName = "model.display_name"
)

// TelemetrySpec declares how a provider reports the telemetry facts, if at all.
// Both transports are optional and independent: a provider may push (Callback),
// be polled (Probe), both, or neither. Absent facts render as absent UI.
type TelemetrySpec struct {
	// Callback is provider-invoked: Crowbar registers a command through
	// config_injection and the provider runs it with a JSON payload on stdin.
	// Same channel and scoping as hooks — runner-scoped, resolved to the chat at
	// ingestion, write-only.
	Callback *TelemetryCallbackSpec `yaml:"callback"`

	// Probe is Crowbar-invoked: a deterministic subcommand run on demand, mapped
	// the same way. It shares the catalogue bounds — timeout, output ceiling,
	// process-group kill — because it is the same class of operation.
	Probe *TelemetryProbeSpec `yaml:"probe"`
}

// TelemetryCallbackSpec maps a pushed payload onto Crowbar facts. Fields maps a
// fact key (the Fact* constants) to a dotted path in the payload; a fact the
// provider does not report is simply absent from the map and stays absent in the
// result. Nothing is derived that was not reported.
type TelemetryCallbackSpec struct {
	Format     string                  `yaml:"format"`
	Fields     map[string]string       `yaml:"fields"`
	RateLimits []TelemetryRateLimitMap `yaml:"rate_limits"`
}

// TelemetryRateLimitMap describes one rate-limit window. Providers disagree on
// how many windows exist and what they are called, so each is declared rather
// than assumed.
type TelemetryRateLimitMap struct {
	ID          string `yaml:"id"`
	Label       string `yaml:"label"`
	UsedPercent string `yaml:"used_percent"`
	ResetsAt    string `yaml:"resets_at"`
}

// TelemetryProbeSpec is a bounded subcommand whose output maps onto the same
// facts. Command omits the executable, exactly like a catalogue pipeline.
type TelemetryProbeSpec struct {
	Format         string            `yaml:"format"`
	Command        []string          `yaml:"command"`
	TimeoutMS      int               `yaml:"timeout_ms"`
	MaxStdoutBytes int               `yaml:"max_stdout_bytes"`
	MaxStderrBytes int               `yaml:"max_stderr_bytes"`
	Fields         map[string]string `yaml:"fields"`
}

func (p *TelemetryProbeSpec) EffectiveTimeoutMS() int {
	if p.TimeoutMS == 0 {
		return DefaultCatalogTimeoutMS
	}
	return p.TimeoutMS
}

func (p *TelemetryProbeSpec) EffectiveMaxStdoutBytes() int {
	if p.MaxStdoutBytes == 0 {
		return DefaultCatalogMaxStdoutBytes
	}
	return p.MaxStdoutBytes
}

func (p *TelemetryProbeSpec) EffectiveMaxStderrBytes() int {
	if p.MaxStderrBytes == 0 {
		return DefaultCatalogMaxStderrBytes
	}
	return p.MaxStderrBytes
}
