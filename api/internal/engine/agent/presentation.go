package agent

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	PromptStrategyRestartTUI = "restart_tui"

	CatalogAdapterJSONTextSection      = "json_text_section"
	CatalogAdapterJSONInventoryDetails = "json_inventory_text_detail"
	CatalogCompletenessComplete        = CatalogCompleteness("complete")
	CatalogCompletenessModelVisible    = CatalogCompleteness("model_visible")
	CatalogCompletenessPluginOnly      = CatalogCompleteness("plugin_only")
	CatalogItemKindSkill               = "skill"
	defaultCatalogTimeoutMS            = 10_000
	defaultCatalogMaxStdoutBytes       = 4 << 20
	defaultCatalogMaxStderrBytes       = 256 << 10
	defaultCatalogMaxItems             = 200
	defaultCatalogDetailConcurrency    = 4
	maxCatalogTimeoutMS                = 10_000
	maxCatalogStdoutBytes              = 4 << 20
	maxCatalogStderrBytes              = 256 << 10
	maxCatalogItems                    = 200
	maxCatalogDetailConcurrency        = 4
)

var ErrPromptSubmitUnsupported = errors.New("agent: provider does not support React prompt submission")

// PresentationSpec is an optional provider-boundary adapter. Domain/usecase/UI
// code consumes the normalized capabilities and never branches on provider id.
type PresentationSpec struct {
	PromptSubmit *PromptSubmitSpec `yaml:"prompt_submit"`
	SlashCatalog *SlashCatalogSpec `yaml:"slash_catalog"`
}

type PromptSubmitSpec struct {
	Strategy string       `yaml:"strategy"`
	Fresh    []InjectStep `yaml:"fresh"`
	Resume   []InjectStep `yaml:"resume"`
}

// CatalogCompleteness says exactly which provider-owned surface a deterministic
// probe can account for. Partial inventories must not be presented as complete.
type CatalogCompleteness string

// SlashCatalogSpec describes a bounded command pipeline. Command arrays omit
// the executable: ProbeSlashCatalog always uses Descriptor.Spawn.Cmd and never a
// shell. Detail templates may interpolate only a parsed inventory {id}, which
// remains one argv element.
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
// {source}, and {id}; a literal '$' before {name} is preserved.
type CatalogItemMapping struct {
	Label       string `yaml:"label"`
	Description string `yaml:"description"`
	InsertText  string `yaml:"insert_text"`
	Source      string `yaml:"source"`
}

// PromptSteps returns a defensive copy of the descriptor steps for a fresh or
// resumed React prompt. The caller sets TemplateCtx.Message and passes these as
// BuildSpawnPlan's extraSteps, preserving the normal config/MCP/session order.
func (d *Descriptor) PromptSteps(resume bool) ([]InjectStep, error) {
	if d == nil || d.Presentation.PromptSubmit == nil {
		return nil, ErrPromptSubmitUnsupported
	}
	steps := d.Presentation.PromptSubmit.Fresh
	if resume {
		steps = d.Presentation.PromptSubmit.Resume
	}
	out := make([]InjectStep, len(steps))
	for i, step := range steps {
		out[i] = InjectStep{Verb: step.Verb, Args: make(map[string]any, len(step.Args))}
		for key, value := range step.Args {
			out[i].Args[key] = value
		}
	}
	return out, nil
}

func (d *Descriptor) validatePresentation() error {
	if d.Presentation.PromptSubmit != nil {
		if err := d.validatePromptSubmit(); err != nil {
			return err
		}
	}
	if d.Presentation.SlashCatalog != nil {
		if err := d.validateSlashCatalog(); err != nil {
			return err
		}
	}
	return nil
}

func (d *Descriptor) validatePromptSubmit() error {
	spec := d.Presentation.PromptSubmit
	if spec.Strategy != PromptStrategyRestartTUI {
		return fmt.Errorf("agent: descriptor %q presentation.prompt_submit has unsupported strategy %q", d.ID, spec.Strategy)
	}
	if d.Session.Resume == nil {
		return fmt.Errorf("agent: descriptor %q presentation.prompt_submit requires session.resume", d.ID)
	}
	for name, steps := range map[string][]InjectStep{"fresh": spec.Fresh, "resume": spec.Resume} {
		if len(steps) == 0 {
			return fmt.Errorf("agent: descriptor %q presentation.prompt_submit.%s is empty", d.ID, name)
		}
		messageCount := 0
		for _, step := range steps {
			if step.Verb != "pass_arg" {
				return fmt.Errorf("agent: descriptor %q presentation.prompt_submit.%s may only pass argv", d.ID, name)
			}
			for _, value := range step.Args {
				messageCount += strings.Count(asString(value), "{message}")
			}
		}
		if messageCount != 1 {
			return fmt.Errorf("agent: descriptor %q presentation.prompt_submit.%s must place {message} exactly once", d.ID, name)
		}
	}
	return nil
}

func (d *Descriptor) validateSlashCatalog() error {
	spec := d.Presentation.SlashCatalog
	if spec.Completeness != CatalogCompletenessComplete &&
		spec.Completeness != CatalogCompletenessModelVisible &&
		spec.Completeness != CatalogCompletenessPluginOnly {
		return fmt.Errorf("agent: descriptor %q presentation.slash_catalog has unsupported completeness %q", d.ID, spec.Completeness)
	}
	if err := validateCatalogBounds(d.ID, spec); err != nil {
		return err
	}
	p := &spec.Pipeline
	if len(p.Command) == 0 || hasEmptyArg(p.Command) {
		return fmt.Errorf("agent: descriptor %q presentation.slash_catalog.pipeline.command must be fixed non-empty argv", d.ID)
	}
	if strings.Contains(strings.Join(p.Command, ""), "{") || strings.Contains(strings.Join(p.Command, ""), "}") {
		return fmt.Errorf("agent: descriptor %q presentation.slash_catalog.pipeline.command must be fixed argv", d.ID)
	}
	for _, arg := range p.Command {
		for _, forbidden := range d.Spawn.ForbidFlags {
			if arg == forbidden {
				return fmt.Errorf("agent: descriptor %q catalog command contains forbidden flag %q", d.ID, forbidden)
			}
		}
	}
	if err := validateItemMapping(d.ID, p.Item); err != nil {
		return err
	}
	switch p.Adapter {
	case CatalogAdapterJSONTextSection:
		if p.TextPath == "" || p.StartMarker == "" || p.EndMarker == "" || p.ItemPattern == "" {
			return fmt.Errorf("agent: descriptor %q json_text_section pipeline is incomplete", d.ID)
		}
		if strings.Contains(p.TextPath, "..") {
			return fmt.Errorf("agent: descriptor %q catalog text_path is invalid", d.ID)
		}
		if err := requireNamedGroup(p.ItemPattern, "name"); err != nil {
			return fmt.Errorf("agent: descriptor %q catalog item_pattern: %w", d.ID, err)
		}
	case CatalogAdapterJSONInventoryDetails:
		if p.RowsPath == "" || p.EnabledField == "" || p.IDField == "" ||
			len(p.DetailCommand) == 0 || hasEmptyArg(p.DetailCommand) ||
			p.DetailPattern == "" || p.DetailItemsGroup == "" || p.DetailSeparator == "" {
			return fmt.Errorf("agent: descriptor %q json_inventory_text_detail pipeline is incomplete", d.ID)
		}
		if countTemplate(p.DetailCommand, "{id}") != 1 {
			return fmt.Errorf("agent: descriptor %q catalog detail_command must place {id} exactly once", d.ID)
		}
		detailTemplates := strings.NewReplacer("{id}", "").Replace(strings.Join(p.DetailCommand, ""))
		if strings.ContainsAny(detailTemplates, "{}") {
			return fmt.Errorf("agent: descriptor %q catalog detail_command has an unsupported template", d.ID)
		}
		for _, arg := range p.DetailCommand {
			for _, forbidden := range d.Spawn.ForbidFlags {
				if arg == forbidden {
					return fmt.Errorf("agent: descriptor %q catalog detail_command contains forbidden flag %q", d.ID, forbidden)
				}
			}
		}
		if err := requireNamedGroup(p.DetailPattern, p.DetailItemsGroup); err != nil {
			return fmt.Errorf("agent: descriptor %q catalog detail_pattern: %w", d.ID, err)
		}
		if p.DetailEmptyPattern != "" {
			if _, err := regexp.Compile(p.DetailEmptyPattern); err != nil {
				return fmt.Errorf("agent: descriptor %q catalog detail_empty_pattern: invalid regex: %w", d.ID, err)
			}
		}
		if p.SourcePattern != "" {
			if err := requireNamedGroup(p.SourcePattern, "source"); err != nil {
				return fmt.Errorf("agent: descriptor %q catalog source_pattern: %w", d.ID, err)
			}
		}
	default:
		return fmt.Errorf("agent: descriptor %q presentation.slash_catalog has unsupported adapter %q", d.ID, p.Adapter)
	}
	return nil
}

func validateCatalogBounds(providerID string, spec *SlashCatalogSpec) error {
	checks := []struct {
		name  string
		value int
		max   int
	}{
		{"timeout_ms", spec.TimeoutMS, maxCatalogTimeoutMS},
		{"max_stdout_bytes", spec.MaxStdoutBytes, maxCatalogStdoutBytes},
		{"max_stderr_bytes", spec.MaxStderrBytes, maxCatalogStderrBytes},
		{"max_items", spec.MaxItems, maxCatalogItems},
		{"pipeline.detail_concurrency", spec.Pipeline.DetailConcurrency, maxCatalogDetailConcurrency},
	}
	for _, check := range checks {
		if check.value < 0 || check.value > check.max {
			return fmt.Errorf("agent: descriptor %q presentation.slash_catalog.%s must be between 0 and %d", providerID, check.name, check.max)
		}
	}
	return nil
}

func validateItemMapping(providerID string, mapping CatalogItemMapping) error {
	if mapping.Label == "" || mapping.InsertText == "" {
		return fmt.Errorf("agent: descriptor %q catalog item mapping requires label and insert_text", providerID)
	}
	for name, value := range map[string]string{
		"label": mapping.Label, "description": mapping.Description,
		"insert_text": mapping.InsertText, "source": mapping.Source,
	} {
		cleaned := strings.NewReplacer("{name}", "", "{description}", "", "{source}", "", "{id}", "").Replace(value)
		if strings.ContainsAny(cleaned, "{}") {
			return fmt.Errorf("agent: descriptor %q catalog item.%s has an unsupported template", providerID, name)
		}
	}
	return nil
}

func requireNamedGroup(pattern, group string) error {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex: %w", err)
	}
	for _, name := range re.SubexpNames() {
		if name == group {
			return nil
		}
	}
	return fmt.Errorf("regex must define named group %q", group)
}

func hasEmptyArg(argv []string) bool {
	for _, arg := range argv {
		if arg == "" {
			return true
		}
	}
	return false
}

func countTemplate(argv []string, template string) int {
	total := 0
	for _, arg := range argv {
		total += strings.Count(arg, template)
	}
	return total
}

func (s *SlashCatalogSpec) effectiveTimeoutMS() int {
	if s.TimeoutMS == 0 {
		return defaultCatalogTimeoutMS
	}
	return s.TimeoutMS
}

func (s *SlashCatalogSpec) effectiveMaxStdoutBytes() int {
	if s.MaxStdoutBytes == 0 {
		return defaultCatalogMaxStdoutBytes
	}
	return s.MaxStdoutBytes
}

func (s *SlashCatalogSpec) effectiveMaxStderrBytes() int {
	if s.MaxStderrBytes == 0 {
		return defaultCatalogMaxStderrBytes
	}
	return s.MaxStderrBytes
}

func (s *SlashCatalogSpec) effectiveMaxItems() int {
	if s.MaxItems == 0 {
		return defaultCatalogMaxItems
	}
	return s.MaxItems
}

func (p *CatalogPipelineSpec) effectiveDetailConcurrency() int {
	if p.DetailConcurrency == 0 {
		return defaultCatalogDetailConcurrency
	}
	return p.DetailConcurrency
}
