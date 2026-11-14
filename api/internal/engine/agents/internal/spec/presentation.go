package spec

const (
	DeliveryRestartTUI = "restart_tui"
)

// PasteBracketed wraps typed text in ESC[200~ ... ESC[201~.
const PasteBracketed = "bracketed"

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

// PromptTypeSpec lets an idle CLI take a prompt typed into its own composer
// instead of a restart. Submit is the key bytes that send it; Composer says how
// the empty input box looks, so the runner can tell it is empty. A message
// opening with a SkipPrefixes entry keeps its meaning under paste (a command or
// a mode switch) and is restarted instead of typed.
type PromptTypeSpec struct {
	Paste        string             `yaml:"paste"`
	Submit       string             `yaml:"submit"`
	SkipPrefixes []string           `yaml:"skip_prefixes"`
	Composer     PromptComposerSpec `yaml:"composer"`
}

// PromptComposerSpec: the composer is empty when the cursor's row starts with
// RowPrefix and the cursor column is exactly that prefix's width.
type PromptComposerSpec struct {
	RowPrefix string `yaml:"row_prefix"`
}

// SteerSpec lets a mid-turn message ride the CLI's turn-end hook instead of a
// restart: Frame wraps {message}, Reply is the hook stdout with {message_json}.
// A message opening with a SkipPrefixes entry is a command and is never steered.
type SteerSpec struct {
	SkipPrefixes []string `yaml:"skip_prefixes"`
	Frame        string   `yaml:"frame"`
	Reply        string   `yaml:"reply"`
}

// LeadingSigilsSpec names the characters this CLI reads as a CONTROL gesture when
// first in a message, and the escape that makes one text. promptsigil applies it
// only where Crowbar's attachment encoding, not a person, put the sigil.
type LeadingSigilsSpec struct {
	Chars  []string `yaml:"chars"`
	Escape string   `yaml:"escape"`
}

type SlashCatalogSpec struct {
	Completeness   CatalogCompleteness `yaml:"completeness"`
	TimeoutMS      int                 `yaml:"timeout_ms"`
	MaxStdoutBytes int                 `yaml:"max_stdout_bytes"`
	MaxStderrBytes int                 `yaml:"max_stderr_bytes"`
	MaxItems       int                 `yaml:"max_items"`
	Pipeline       CatalogPipelineSpec `yaml:"pipeline"`
	// Builtins are the CLI's own commands, which no probe reports.
	Builtins []CatalogBuiltin `yaml:"builtins"`
	// Workspace names the directories, inside the workspace, the CLI reads commands from.
	Workspace []CatalogWorkspaceDir `yaml:"workspace"`
}

// CatalogBuiltin is one command the CLI ships. Terminal marks one that opens a
// dialog only the CLI's own terminal can answer.
type CatalogBuiltin struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Terminal    bool   `yaml:"terminal"`
}

// CatalogWorkspaceDir is a directory of commands relative to the workspace root:
// each *.md file is one, or with Entry, each subdirectory holding that file is a skill.
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
