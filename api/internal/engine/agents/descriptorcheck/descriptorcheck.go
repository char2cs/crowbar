// Package descriptorcheck validates a provider descriptor before Crowbar runs
// it: static rules over the document (Validate) and a live conformance run
// against the real CLI (Conform). A descriptor with an error-severity finding
// is never enabled.
package descriptorcheck

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Severity says whether a finding blocks the descriptor.
type Severity string

const (
	// SeverityError blocks enabling the descriptor.
	SeverityError Severity = "error"
	// SeverityWarning is shown but does not block.
	SeverityWarning Severity = "warning"
)

// Finding is one problem with a descriptor document.
type Finding struct {
	Rule     string   `json:"rule"`
	Severity Severity `json:"severity"`
	// Path is the YAML path the finding is about, e.g. "session.locate.glob[0]".
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Report is every finding for one descriptor document.
type Report struct {
	ID string `json:"id"`
	// Source is the override file validated, or "" for the embedded default.
	Source   string    `json:"source,omitempty"`
	Findings []Finding `json:"findings"`
	// FellBack says the override at Source was refused for its error
	// findings and the shipped descriptor runs in its place.
	FellBack bool `json:"fellBack,omitempty"`
}

// OK reports whether nothing blocks the descriptor.
func (r Report) OK() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return false
		}
	}
	return true
}

// Validate runs every static rule over one descriptor document.
func Validate(raw []byte) Report {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return Report{Findings: []Finding{syntaxFinding(err)}}
	}
	doc := document{root: &root}
	d, failures := protocol.CheckDescriptor(raw)
	rep := Report{ID: probeID(&root)}
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
	}
	sort.SliceStable(rep.Findings, func(i, j int) bool {
		return rep.Findings[i].Line < rep.Findings[j].Line
	})
	return rep
}

// Source is one descriptor document Crowbar would load: ID, Path (the
// override file, "" for the shipped default) and Raw.
type Source = protocol.DescriptorSource

// Sources lists every descriptor document under homeDir, overrides shadowing
// the shipped defaults exactly as the daemon resolves them.
func Sources(homeDir string) ([]Source, error) {
	sources, err := protocol.DescriptorSources(homeDir)
	if err != nil {
		return nil, fmt.Errorf("descriptorcheck: %w", err)
	}
	return sources, nil
}

// ValidateAll validates every descriptor Crowbar would load from homeDir. An
// override refused in favour of the shipped default reports its own findings,
// marked FellBack, so the user sees why their file is not the one running.
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

// AcceptOverride admits an override the daemon may run: one with no
// error-severity finding. It is the predicate agents.WithOverrideCheck loads
// descriptors through, so what runs and what the gate checks agree.
func AcceptOverride(raw []byte) bool {
	return Validate(raw).OK()
}

// fallsBack reports whether src is a refused override with a shipped default
// to run instead.
func fallsBack(src Source, rep Report) bool {
	if src.Path == "" || rep.OK() {
		return false
	}
	_, ok := protocol.EmbeddedDescriptorSource(src.ID)
	return ok
}

func syntaxFinding(err error) Finding {
	return Finding{
		Rule: "yaml.syntax", Severity: SeverityError, Line: errorLine(err.Error()),
		Message: err.Error(), Hint: "fix the YAML syntax; nothing else can be checked until it parses",
	}
}

// unknownFields decodes strictly: an unknown key is a typo that the lenient
// load would silently ignore (a misspelt hooks_injection wires no hooks).
func unknownFields(raw []byte, doc document) []Finding {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var d spec.Descriptor
	err := dec.Decode(&d)
	var typeErr *yaml.TypeError
	if !errors.As(err, &typeErr) {
		return nil
	}
	out := make([]Finding, 0, len(typeErr.Errors))
	for _, msg := range typeErr.Errors {
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
