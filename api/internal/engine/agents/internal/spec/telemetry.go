package spec

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

type TelemetrySpec struct {
	Callback *TelemetryCallbackSpec `yaml:"callback"`

	Probe *TelemetryProbeSpec `yaml:"probe"`
}

type TelemetryCallbackSpec struct {
	Format     string                  `yaml:"format"`
	Fields     map[string]string       `yaml:"fields"`
	RateLimits []TelemetryRateLimitMap `yaml:"rate_limits"`
}

type TelemetryRateLimitMap struct {
	ID          string `yaml:"id"`
	Label       string `yaml:"label"`
	UsedPercent string `yaml:"used_percent"`
	ResetsAt    string `yaml:"resets_at"`
}

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
