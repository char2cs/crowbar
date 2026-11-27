package spec

const (
	DeliveryRestartTUI = "restart_tui"
)

const InputKeys = "keys"

const (
	CatalogAdapterJSONTextSection      = "json_text_section"
	CatalogAdapterJSONInventoryDetails = "json_inventory_text_detail"
)

type CatalogCompleteness string

const (
	CatalogCompletenessComplete     = CatalogCompleteness("complete")
	CatalogCompletenessModelVisible = CatalogCompleteness("model_visible")
	CatalogCompletenessPluginOnly   = CatalogCompleteness("plugin_only")
)

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

type PresentationSpec struct {
	PromptSubmit *PromptSubmitSpec `yaml:"prompt_submit"`
	SlashCatalog *SlashCatalogSpec `yaml:"slash_catalog"`
}

type PromptSubmitSpec struct {
	Strategy      string             `yaml:"strategy"`
	Fresh         InjectSteps        `yaml:"fresh"`
	Resume        InjectSteps        `yaml:"resume"`
	LeadingSigils *LeadingSigilsSpec `yaml:"leading_sigils"`
	Steer         *SteerSpec         `yaml:"steer"`
	Type          *PromptTypeSpec    `yaml:"type"`
}

type PromptTypeSpec struct {
	Input           string             `yaml:"input"`
	Submit          string             `yaml:"submit"`
	ChunkBytes      int                `yaml:"chunk_bytes"`
	ChunkIntervalMS int                `yaml:"chunk_interval_ms"`
	SkipPrefixes    []string           `yaml:"skip_prefixes"`
	SkipSuffixes    []string           `yaml:"skip_suffixes"`
	Composer        PromptComposerSpec `yaml:"composer"`
}

type PromptComposerSpec struct {
	RowPrefix string `yaml:"row_prefix"`
	Ghost     string `yaml:"ghost"`
}

const GhostFaint = "faint"

type SteerSpec struct {
	SkipPrefixes []string `yaml:"skip_prefixes"`
	Frame        string   `yaml:"frame"`
	Reply        string   `yaml:"reply"`
}

type LeadingSigilsSpec struct {
	Chars  []string `yaml:"chars"`
	Escape string   `yaml:"escape"`
}

type SlashCatalogSpec struct {
	Completeness   CatalogCompleteness   `yaml:"completeness"`
	TimeoutMS      int                   `yaml:"timeout_ms"`
	MaxStdoutBytes int                   `yaml:"max_stdout_bytes"`
	MaxStderrBytes int                   `yaml:"max_stderr_bytes"`
	MaxItems       int                   `yaml:"max_items"`
	Pipeline       CatalogPipelineSpec   `yaml:"pipeline"`
	Builtins       []CatalogBuiltin      `yaml:"builtins"`
	Workspace      []CatalogWorkspaceDir `yaml:"workspace"`
}

type CatalogBuiltin struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Terminal    bool   `yaml:"terminal"`
}

type CatalogWorkspaceDir struct {
	Dir    string `yaml:"dir"`
	Entry  string `yaml:"entry"`
	Source string `yaml:"source"`
}

type CatalogPipelineSpec struct {
	Adapter string   `yaml:"adapter"`
	Command []string `yaml:"command"`

	TextPath    string `yaml:"text_path"`
	StartMarker string `yaml:"start_marker"`
	EndMarker   string `yaml:"end_marker"`
	ItemPattern string `yaml:"item_pattern"`

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
