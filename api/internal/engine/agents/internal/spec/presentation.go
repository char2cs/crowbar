package spec

// Prompt-delivery strategies. RestartTUI is the portable floor and must always
// work; anything else is an optimisation a descriptor edit can withdraw without
// a code change.
const (
	DeliveryRestartTUI = "restart_tui"
	DeliveryRewakeHook = "rewake_hook"
)

// Catalogue adapters, named for the SHAPE of the output they read rather than
// for the provider that happens to produce it.
const (
	CatalogAdapterJSONTextSection      = "json_text_section"
	CatalogAdapterJSONInventoryDetails = "json_inventory_text_detail"
)

// CatalogCompleteness says exactly which provider-owned surface a deterministic
// probe can account for. A partial inventory must never be presented as
// complete.
type CatalogCompleteness string

const (
	CatalogCompletenessComplete     = CatalogCompleteness("complete")
	CatalogCompletenessModelVisible = CatalogCompleteness("model_visible")
	CatalogCompletenessPluginOnly   = CatalogCompleteness("plugin_only")
)

// Bounds every catalogue probe is held to. They are ceilings, not defaults: a
// descriptor may ask for less and never for more, so a descriptor edit cannot
// widen the engine's exposure to a provider command.
const (
	DefaultCatalogTimeoutMS         = 10_000
	DefaultCatalogMaxStdoutBytes    = 4 << 20
	DefaultCatalogMaxStderrBytes    = 256 << 10
	DefaultCatalogMaxItems          = 200
	DefaultCatalogDetailConcurrency = 4

	MaxCatalogTimeoutMS         = 10_000
	MaxCatalogStdoutBytes       = 4 << 20
	MaxCatalogStderrBytes       = 256 << 10
	MaxCatalogItems             = 200
	MaxCatalogDetailConcurrency = 4
)

// PresentationSpec is an optional provider-boundary adapter. Domain, usecase and
// UI code consume the normalised capabilities and never branch on provider id.
type PresentationSpec struct {
	PromptSubmit *PromptSubmitSpec `yaml:"prompt_submit"`
	SlashCatalog *SlashCatalogSpec `yaml:"slash_catalog"`
}

type PromptSubmitSpec struct {
	Strategy string       `yaml:"strategy"`
	Fresh    []InjectStep `yaml:"fresh"`
	Resume   []InjectStep `yaml:"resume"`

	// Rewake configures DeliveryRewakeHook. Required by that strategy and
	// ignored by any other.
	Rewake *RewakeSpec `yaml:"rewake"`
}

// RewakeSpec describes a push-free delivery channel: the provider runs a Crowbar
// command in the background and collects a queued prompt from it, so an
// ambiguous outcome is resolved by asking rather than by guessing.
//
// Wrapper is the text the provider wraps the collected prompt in before it
// reaches the user-prompt hook. Both halves are Crowbar-controlled, and Sentinel
// doubles as the discriminator for "Crowbar delivered this" — necessary because,
// unlike a restart, the same runner emits both native and injected prompts.
type RewakeSpec struct {
	Sentinel     string   `yaml:"sentinel"`
	StripPrefix  []string `yaml:"strip_prefix"`
	StripSuffix  []string `yaml:"strip_suffix"`
	StripPattern string   `yaml:"strip_pattern"`
}

// SlashCatalogSpec describes a bounded command pipeline. Command arrays omit the
// executable: a probe always uses Descriptor.Spawn.Cmd and never a shell. Detail
// templates may interpolate only a parsed inventory {id}, which remains one argv
// element.
type SlashCatalogSpec struct {
	Completeness   CatalogCompleteness `yaml:"completeness"`
	TimeoutMS      int                 `yaml:"timeout_ms"`
	MaxStdoutBytes int                 `yaml:"max_stdout_bytes"`
	MaxStderrBytes int                 `yaml:"max_stderr_bytes"`
	MaxItems       int                 `yaml:"max_items"`
	Pipeline       CatalogPipelineSpec `yaml:"pipeline"`
}

// CatalogPipelineSpec is deliberately output-shape based rather than provider
// based. Fields unused by the selected adapter must remain empty.
type CatalogPipelineSpec struct {
	Adapter string   `yaml:"adapter"`
	Command []string `yaml:"command"`

	// json_text_section fields.
	TextPath    string `yaml:"text_path"`
	StartMarker string `yaml:"start_marker"`
	EndMarker   string `yaml:"end_marker"`
	ItemPattern string `yaml:"item_pattern"`

	// json_inventory_text_detail fields.
	RowsPath           string   `yaml:"rows_path"`
	EnabledField       string   `yaml:"enabled_field"`
	IDField            string   `yaml:"id_field"`
	SourcePattern      string   `yaml:"source_pattern"`
	DetailCommand      []string `yaml:"detail_command"`
	DetailPattern      string   `yaml:"detail_pattern"`
	DetailEmptyPattern string   `yaml:"detail_empty_pattern"`
	DetailItemsGroup   string   `yaml:"detail_items_group"`
	DetailSeparator    string   `yaml:"detail_separator"`
	DetailConcurrency  int      `yaml:"detail_concurrency"`

	Item CatalogItemMapping `yaml:"item"`
}

// CatalogItemMapping maps named regex captures and inventory values to the
// provider-neutral result. Supported placeholders are {name}, {description},
// {source} and {id}.
type CatalogItemMapping struct {
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	InsertText  string `yaml:"insert_text"`
	Source      string `yaml:"source"`
}

func (s *SlashCatalogSpec) EffectiveTimeoutMS() int {
	if s.TimeoutMS == 0 {
		return DefaultCatalogTimeoutMS
	}
	return s.TimeoutMS
}

func (s *SlashCatalogSpec) EffectiveMaxStdoutBytes() int {
	if s.MaxStdoutBytes == 0 {
		return DefaultCatalogMaxStdoutBytes
	}
	return s.MaxStdoutBytes
}

func (s *SlashCatalogSpec) EffectiveMaxStderrBytes() int {
	if s.MaxStderrBytes == 0 {
		return DefaultCatalogMaxStderrBytes
	}
	return s.MaxStderrBytes
}

func (s *SlashCatalogSpec) EffectiveMaxItems() int {
	if s.MaxItems == 0 {
		return DefaultCatalogMaxItems
	}
	return s.MaxItems
}

func (p *CatalogPipelineSpec) EffectiveDetailConcurrency() int {
	if p.DetailConcurrency == 0 {
		return DefaultCatalogDetailConcurrency
	}
	return p.DetailConcurrency
}
