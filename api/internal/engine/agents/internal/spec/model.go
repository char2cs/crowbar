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
	// Apply renders onto the argv of a process the spawn FORKS. APIApply
	// renders onto the api channel's own serve process instead — an
	// api-transport spawn whose connection comes up forks nothing at all, so
	// Apply reaches no process and the choice is silently dropped. Both are
	// required of an api-transport descriptor (rules.selectionAPICarrier).
	Apply    []InjectStep `yaml:"apply"`
	APIApply []InjectStep `yaml:"api_apply"`
}

// ModelManifestSpec resolves a model catalogue from a JSON document instead
// of a live probe — for a provider with no enumerable model surface to run
// a command against at all. The document is fetched over the network,
// cached on disk, and bundled into the binary as a last resort;
// resolution picks whichever candidate's own "updatedAt" is freshest — see
// modeldiscovery.ProbeManifest. ItemsPath/KeepWhen/Item share
// ModelDiscoverSpec's exact vocabulary (Item.Efforts included: a manifest
// row states its OWN model's effort levels, exactly like a discover: row
// does); rules.modelCatalog rejects a descriptor declaring more than one of
// available:/discover:/manifest:.
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

// ModelDiscoverSpec asks a live provider what models it currently offers
// instead of a hand-maintained available: list this Go side would drift from
// the moment the provider adds or retires one. Every field name below is
// DATA — a template placeholder or a field path into whatever JSON the
// command prints — read generically; rules.modelCatalog rejects a
// descriptor declaring both available: and discover:.
type ModelDiscoverSpec struct {
	Command   []string         `yaml:"command"`
	Adapter   string           `yaml:"adapter"`
	ItemsPath string           `yaml:"items_path"`
	KeepWhen  *ModelFieldMatch `yaml:"keep_when"`
	OrderBy   string           `yaml:"order_by"`
	// DefaultWhen flags the row that IS this provider's own default model —
	// nil when the source states no such thing (a per-model effort default
	// is NOT a model default; the two must never be conflated). Absent
	// DefaultWhen, or no row matching it, means the default is UNKNOWN —
	// never inferred from list position or order_by.
	DefaultWhen    *ModelFieldMatch `yaml:"default_when"`
	Item           ModelItemMapping `yaml:"item"`
	TimeoutMS      int              `yaml:"timeout_ms"`
	MaxStdoutBytes int              `yaml:"max_stdout_bytes"`
}

// ModelFieldMatch is one field/value equality test against a discovered row —
// keep_when's filter and default_when's flag share the same shape. Equals is
// `any` because a source's own value can be a string or a bool depending on
// the field it names.
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
