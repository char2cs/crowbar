package spec

const (
	ModelDiscoverAdapterJSON = "json"

	DefaultModelDiscoverTimeoutMS      = 10_000
	MaxModelDiscoverTimeoutMS          = 30_000
	DefaultModelDiscoverMaxStdoutBytes = 4 << 20
	MaxModelDiscoverMaxStdoutBytes     = 4 << 20

	DefaultModelManifestTimeoutMS = 10_000
	MaxModelManifestTimeoutMS     = 10_000
	DefaultModelManifestTTLMS     = 3_600_000
	MaxModelManifestTTLMS         = 24 * 60 * 60 * 1000
)

type ModelSpec struct {
	Available []string           `yaml:"available"`
	Discover  *ModelDiscoverSpec `yaml:"discover"`
	Manifest  *ModelManifestSpec `yaml:"manifest"`
	Strategy  string             `yaml:"strategy"`
	Apply     InjectSteps        `yaml:"apply"`
	APIApply  InjectSteps        `yaml:"api_apply"`
	IDSplit   *ModelIDSplit      `yaml:"id_split"`
}

type ModelIDSplit struct {
	Sep   string   `yaml:"sep"`
	Names []string `yaml:"names"`
}

type ModelManifestSpec struct {
	URL       string           `yaml:"url"`
	ItemsPath string           `yaml:"items_path"`
	KeepWhen  *ModelFieldMatch `yaml:"keep_when"`
	Item      ModelItemMapping `yaml:"item"`
	TTLMS     int              `yaml:"ttl_ms"`
	TimeoutMS int              `yaml:"timeout_ms"`
}

func (s *ModelManifestSpec) EffectiveTimeoutMS() int {
	if s.TimeoutMS == 0 {
		return DefaultModelManifestTimeoutMS
	}
	return s.TimeoutMS
}

func (s *ModelManifestSpec) EffectiveTTLMS() int {
	if s.TTLMS == 0 {
		return DefaultModelManifestTTLMS
	}
	return s.TTLMS
}

type ModelDiscoverSpec struct {
	Command        []string          `yaml:"command"`
	API            *ModelDiscoverAPI `yaml:"api"`
	Adapter        string            `yaml:"adapter"`
	ItemsPath      string            `yaml:"items_path"`
	KeepWhen       *ModelFieldMatch  `yaml:"keep_when"`
	OrderBy        string            `yaml:"order_by"`
	DefaultWhen    *ModelFieldMatch  `yaml:"default_when"`
	CurrentID      string            `yaml:"current_id"`
	Item           ModelItemMapping  `yaml:"item"`
	TimeoutMS      int               `yaml:"timeout_ms"`
	MaxStdoutBytes int               `yaml:"max_stdout_bytes"`
}

type ModelDiscoverAPI struct {
	Call   string         `yaml:"call"`
	Params map[string]any `yaml:"params"`
}

type ModelFieldMatch struct {
	Field  string `yaml:"field"`
	Equals any    `yaml:"equals"`
}

type ModelItemMapping struct {
	Each          string `yaml:"each"`
	ID            string `yaml:"id"`
	Label         string `yaml:"label"`
	Efforts       string `yaml:"efforts"`
	DefaultEffort string `yaml:"default_effort"`
}

func (s *ModelDiscoverSpec) EffectiveTimeoutMS() int {
	if s.TimeoutMS == 0 {
		return DefaultModelDiscoverTimeoutMS
	}
	return s.TimeoutMS
}

func (s *ModelDiscoverSpec) EffectiveMaxStdoutBytes() int {
	if s.MaxStdoutBytes == 0 {
		return DefaultModelDiscoverMaxStdoutBytes
	}
	return s.MaxStdoutBytes
}
