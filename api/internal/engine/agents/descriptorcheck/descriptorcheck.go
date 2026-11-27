package descriptorcheck

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptorschema"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
)

type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	Path     string   `json:"path"`
	Line     int      `json:"line"`
	Message  string   `json:"message"`
	Hint     string   `json:"hint,omitempty"`
}

type Report struct {
	ID       string    `json:"id"`
	Source   string    `json:"source,omitempty"`
	Findings []Finding `json:"findings"`
	FellBack bool      `json:"fellBack,omitempty"`
}

func (r Report) OK() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return false
		}
	}
	return true
}

func Validate(raw []byte) Report {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return Report{Findings: []Finding{syntaxFinding(err)}}
	}
	if err := protocol.CheckDescriptorVersion(raw); err != nil {
		return Report{ID: probeID(&root), Findings: []Finding{schemaFinding(document{root: &root}, err)}}
	}
	doc := document{root: &root}
	d, failures := protocol.CheckDescriptor(raw)
	rep := Report{ID: probeID(&root), Findings: []Finding{}}
	rep.Findings = append(rep.Findings, unknownFields(raw, doc)...)
	for _, f := range failures {
		rep.Findings = append(rep.Findings, loadFinding(doc, f))
	}
	if d != nil {
		rep.Findings = append(rep.Findings, sessionRules(doc, d)...)
		rep.Findings = append(rep.Findings, templateRules(doc, d)...)
		rep.Findings = append(rep.Findings, channelRules(doc, d)...)
		rep.Findings = append(rep.Findings, safetyRules(doc, d)...)
		rep.Findings = append(rep.Findings, lifecycleRules(doc, d)...)
		rep.Findings = append(rep.Findings, noticeRules(doc, d)...)
	}
	sort.SliceStable(rep.Findings, func(i, j int) bool {
		return rep.Findings[i].Line < rep.Findings[j].Line
	})
	return rep
}

type Source = protocol.DescriptorSource

func Sources(homeDir string) ([]Source, error) {
	sources, err := protocol.DescriptorSources(homeDir)
	if err != nil {
		return nil, fmt.Errorf("descriptorcheck: %w", err)
	}
	return sources, nil
}

func ValidateAll(homeDir string) ([]Report, error) {
	sources, err := Sources(homeDir)
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(sources))
	for _, src := range sources {
		rep := Validate(src.Raw)
		if rep.ID == "" {
			rep.ID = src.ID
		}
		rep.Source = src.Path
		rep.FellBack = fallsBack(src, rep)
		out = append(out, rep)
	}
	return out, nil
}

func AcceptOverride(raw []byte) bool {
	return Validate(raw).OK()
}

func fallsBack(src Source, rep Report) bool {
	if src.Path == "" || rep.OK() {
		return false
	}
	_, ok := protocol.EmbeddedDescriptorSource(src.ID)
	return ok
}

func schemaFinding(doc document, err error) Finding {
	return Finding{
		Rule: "schema.version", Severity: SeverityError, Path: descriptorschema.VersionKey,
		Line: doc.line(descriptorschema.VersionKey), Message: err.Error(),
		Hint: fmt.Sprintf("the descriptor schema is pre-release and changes without migration: rewrite the file "+
			"in the shape of the shipped descriptors and set %s: %d, or remove it", descriptorschema.VersionKey,
			descriptorschema.Current),
	}
}

func syntaxFinding(err error) Finding {
	return Finding{
		Rule: "yaml.syntax", Severity: SeverityError, Line: errorLine(err.Error()),
		Message: err.Error(), Hint: "fix the YAML syntax; nothing else can be checked until it parses",
	}
}

func unknownFields(raw []byte, doc document) []Finding {
	keys := protocol.UnknownDescriptorKeys(raw)
	out := make([]Finding, 0, len(keys))
	for _, msg := range keys {
		line := errorLine(msg)
		out = append(out, Finding{
			Rule: "yaml.unknown_field", Severity: SeverityError, Line: line,
			Path: doc.pathAt(line), Message: msg,
			Hint: "remove the key or correct its spelling; unknown keys are otherwise ignored",
		})
	}
	return out
}

func probeID(root *yaml.Node) string {
	if n := (document{root: root}).lookup("id"); n != nil {
		return n.Value
	}
	return ""
}
