package spec

const (
	ModelDiscoverAdapterJSON = "json"

	DefaultModelDiscoverTimeoutMS      = 10_000
	MaxModelDiscoverTimeoutMS          = 10_000
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
	// Apply renders onto the argv of a process the spawn FORKS; APIApply onto the api
	// serve process, since an api spawn forks nothing for Apply to reach. An api
	// descriptor needs both (rules.selectionAPICarrier).
	Apply    InjectSteps `yaml:"apply"`
	APIApply InjectSteps `yaml:"api_apply"`
}

// ModelManifestSpec resolves a model catalogue from a JSON document (fetched, cached,
// bundled; freshest "updatedAt" wins) for a provider with no command to probe. It
// shares ModelDiscoverSpec's row vocabulary; only one catalogue source is allowed.
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

// ModelDiscoverSpec asks a live provider which models it offers, so no
// hand-maintained list drifts. Every field name is DATA (a template or a path into
// the command's JSON); rules.modelCatalog allows only one catalogue source.
type ModelDiscoverSpec struct {
	Command   []string         `yaml:"command"`
	Adapter   string           `yaml:"adapter"`
	ItemsPath string           `yaml:"items_path"`
	KeepWhen  *ModelFieldMatch `yaml:"keep_when"`
	OrderBy   string           `yaml:"order_by"`
	// DefaultWhen flags the row that IS the provider's default model (not an effort
	// default). Absent, or matching no row, the default is UNKNOWN, never positional.
	DefaultWhen    *ModelFieldMatch `yaml:"default_when"`
	Item           ModelItemMapping `yaml:"item"`
	TimeoutMS      int              `yaml:"timeout_ms"`
	MaxStdoutBytes int              `yaml:"max_stdout_bytes"`
}

// ModelFieldMatch is one field/value equality test on a discovered row (keep_when,
// default_when). Equals is `any` because the value may be a string or a bool.
type ModelFieldMatch struct {
	Field  string `yaml:"field"`
	Equals any    `yaml:"equals"`
}

// ModelItemMapping renders one kept row into a model. ID/Label/DefaultEffort
// are `{field}` templates; Efforts is a pathselect expression (typically
// ending "[].effort") yielding that row's own effort list.
type ModelItemMapping struct {
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
