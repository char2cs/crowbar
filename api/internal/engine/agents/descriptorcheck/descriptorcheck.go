// Package descriptorcheck validates a provider descriptor before Crowbar runs it: static rules
// over the document (Validate) and a live conformance run against the real CLI (Conform). A
// descriptor with an error-severity finding is never enabled.
package descriptorcheck

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptorschema"
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
	// Path is the YAML path the finding is about, e.g. "session.refused.needles[0]".
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

// Validate runs every static rule over one descriptor document, brought up to
// the current schema first; what that upgrade changed is reported as warnings.
func Validate(raw []byte) Report {
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		return Report{Findings: []Finding{syntaxFinding(err)}}
	}
	up, err := descriptorschema.Upgrade(raw)
	if err != nil {
		return Report{ID: probeID(&root), Findings: []Finding{schemaFinding(err)}}
	}
	if len(up.Changes) > 0 {
		raw, root = up.Raw, yaml.Node{}
		if err := yaml.Unmarshal(raw, &root); err != nil {
			return Report{Findings: []Finding{syntaxFinding(err)}}
		}
	}
	doc := document{root: &root}
	d, failures := protocol.CheckDescriptor(raw)
	rep := Report{ID: probeID(&root)}
	rep.Findings = append(rep.Findings, changeFindings(up)...)
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
	graceOlderSchema(rep.Findings, up.From)
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

// Upgraded is a descriptor document brought up to the current schema.
type Upgraded = descriptorschema.Result

// Upgrade brings raw up to the current descriptor schema, for a caller that
// rewrites an override on disk; Validate and the loader apply it themselves.
// The result states the newest schema its content meets, so a file still short
// of a later requirement keeps running as the older document it is.
func Upgrade(raw []byte) (Upgraded, error) {
	up, err := descriptorschema.Upgrade(raw)
	if err != nil {
		return up, fmt.Errorf("descriptorcheck: %w", err)
	}
	if len(up.Changes) == 0 {
		return up, nil
	}
	meets := descriptorschema.Current
	for _, f := range Validate(up.Raw).Findings {
		if since, ok := laterRequirements[f.Rule]; ok && f.Severity == SeverityError && since > up.From {
			meets = min(meets, since-1)
		}
	}
	if meets == descriptorschema.Current {
		return up, nil
	}
	if up.Raw, err = descriptorschema.Stamp(up.Raw, meets); err != nil {
		return up, fmt.Errorf("descriptorcheck: %w", err)
	}
	return up, nil
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

// changeFindings warns of each thing the schema upgrade did, at the line of the
// user's own file: a retired key is ignored, a rewritten one runs as its new shape.
func changeFindings(up descriptorschema.Result) []Finding {
	hint := fmt.Sprintf("Crowbar rewrites the file this way at its next start and keeps the original beside it; "+
		"or make the change yourself and set %s: %d", descriptorschema.VersionKey, descriptorschema.Current)
	out := make([]Finding, 0, len(up.Changes))
	for _, c := range up.Changes {
		rule := "schema.migrated"
		if c.Kind == descriptorschema.Retired {
			rule = "schema.retired_key"
		}
		out = append(out, Finding{
			Rule: rule, Severity: SeverityWarning, Path: c.Path, Line: c.Line,
			Message: fmt.Sprintf("brought up to schema %d: %s", descriptorschema.Current, c.Note), Hint: hint,
		})
	}
	return out
}

// laterRequirements are the facts a schema began to require, by that schema:
// without them the provider runs, short of what each one enables, so a document
// written before they existed is warned of them, not blocked.
var laterRequirements = map[string]int{
	"session.refusal_unrecognised":    3,
	"session.refused_rpc":             3,
	"session.refused_exit":            3,
	"session.refused_needles":         3,
	"terminal_notices.anchor_missing": 3,
}

func graceOlderSchema(findings []Finding, from int) {
	for i, f := range findings {
		since, ok := laterRequirements[f.Rule]
		if !ok || f.Severity != SeverityError || from >= since {
			continue
		}
		findings[i].Severity = SeverityWarning
		findings[i].Message += fmt.Sprintf(" (schema %d asks for this; the file predates it, so the provider runs without it)", since)
		findings[i].Hint += fmt.Sprintf("; then set %s: %d", descriptorschema.VersionKey, since)
	}
}

func schemaFinding(err error) Finding {
	return Finding{
		Rule: "schema.version", Severity: SeverityError, Path: descriptorschema.VersionKey, Message: err.Error(),
		Hint: fmt.Sprintf("this build reads descriptor schemas up to %d: update Crowbar or use a descriptor written for it",
			descriptorschema.Current),
	}
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
