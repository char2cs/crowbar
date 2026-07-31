package gen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Manifest is the JSON record of one protogen run: every endpoint with its
// resolved DTOs, every emitted type, and — the part that matters most — every
// thing that did not resolve, with the structural reason.
type Manifest struct {
	// Generator names the tool, so a stale manifest is identifiable.
	Generator string `json:"generator"`
	// Stats are the coverage numbers.
	Stats Stats `json:"stats"`
	// Endpoints is every discovered route.
	Endpoints []Endpoint `json:"endpoints"`
	// Types is every emitted declaration.
	Types []ManifestType `json:"types"`
	// Unresolved is everything protogen refused to guess at.
	Unresolved []Unresolved `json:"unresolved"`
	// UnresolvedByCategory counts the unresolved list by structural cause.
	UnresolvedByCategory []CategoryCount `json:"unresolvedByCategory"`
}

// ManifestType is one emitted declaration's identity, without its fields.
type ManifestType struct {
	// Module is the emitted module.
	Module string `json:"module"`
	// Name is the emitted name.
	Name string `json:"name"`
	// Kind is struct, enum or alias.
	Kind Kind `json:"kind"`
	// GoPath is the Go type it came from.
	GoPath string `json:"goPath"`
	// Fields counts the emitted fields (structs only).
	Fields int `json:"fields,omitempty"`
	// Dropped lists the fields left out of this declaration; a non-empty
	// Dropped means the emitted type is NOT the full wire shape.
	Dropped []Unresolved `json:"dropped,omitempty"`
	// Variants counts the emitted variants (enums only).
	Variants int `json:"variants,omitempty"`
	// Synthetic marks a name protogen invented for an anonymous Go struct.
	Synthetic bool `json:"synthetic,omitempty"`
}

// CategoryCount is one bucket of the unresolved summary.
type CategoryCount struct {
	// Category is the structural cause.
	Category string `json:"category"`
	// Count is how many items fell into it.
	Count int `json:"count"`
}

// Stats are the coverage numbers a run produced.
type Stats struct {
	// Endpoints is the number of (method, path) routes discovered.
	Endpoints int `json:"endpoints"`
	// DistinctPaths is the number of distinct paths across those routes.
	DistinctPaths int `json:"distinctPaths"`
	// HandlersResolved is how many routes resolved to a handler declaration.
	HandlersResolved int `json:"handlersResolved"`
	// FullyResolved is how many endpoints had nothing left unresolved.
	FullyResolved int `json:"fullyResolved"`
	// WithRequestBody is how many endpoints bind a request body.
	WithRequestBody int `json:"withRequestBody"`
	// RequestResolved is how many of those resolved it.
	RequestResolved int `json:"requestResolved"`
	// WithResponsePayload is how many endpoints write a JSON payload.
	WithResponsePayload int `json:"withResponsePayload"`
	// ResponseResolved is how many of those resolved every payload type.
	ResponseResolved int `json:"responseResolved"`
	// ByResponseKind counts endpoints per response classification.
	ByResponseKind map[string]int `json:"byResponseKind"`
	// Types is the number of emitted declarations.
	Types int `json:"types"`
	// TypesByKind counts declarations per kind.
	TypesByKind map[string]int `json:"typesByKind"`
	// TypesIncomplete counts declarations missing at least one field.
	TypesIncomplete int `json:"typesIncomplete"`
	// Modules is the number of emitted modules.
	Modules int `json:"modules"`
	// Unresolved is the total size of the diagnostics list.
	Unresolved int `json:"unresolved"`
	// UnresolvedErrors counts the diagnostics for which nothing was emitted.
	UnresolvedErrors int `json:"unresolvedErrors"`
	// UnresolvedWarnings counts the best-effort emissions to confirm by hand.
	UnresolvedWarnings int `json:"unresolvedWarnings"`
}

// BuildManifest folds a result into its manifest form.
func BuildManifest(
	r *Result,
) Manifest {
	m := Manifest{
		Generator:  "native/tools/protogen",
		Endpoints:  r.Endpoints,
		Unresolved: r.Unresolved,
		Stats:      ComputeStats(r),
	}
	for _, d := range r.Decls {
		m.Types = append(m.Types, ManifestType{
			Module:    d.Module,
			Name:      d.Name,
			Kind:      d.Kind,
			GoPath:    d.GoPath,
			Fields:    len(d.Fields),
			Dropped:   d.Dropped,
			Variants:  len(d.Variants),
			Synthetic: d.Synthetic,
		})
	}
	counts := map[string]int{}
	for _, u := range r.Unresolved {
		counts[u.Category]++
	}
	for cat, n := range counts {
		m.UnresolvedByCategory = append(m.UnresolvedByCategory, CategoryCount{Category: cat, Count: n})
	}
	sort.SliceStable(m.UnresolvedByCategory, func(i, j int) bool {
		if m.UnresolvedByCategory[i].Count != m.UnresolvedByCategory[j].Count {
			return m.UnresolvedByCategory[i].Count > m.UnresolvedByCategory[j].Count
		}
		return m.UnresolvedByCategory[i].Category < m.UnresolvedByCategory[j].Category
	})
	if m.Endpoints == nil {
		m.Endpoints = []Endpoint{}
	}
	if m.Types == nil {
		m.Types = []ManifestType{}
	}
	if m.Unresolved == nil {
		m.Unresolved = []Unresolved{}
	}
	if m.UnresolvedByCategory == nil {
		m.UnresolvedByCategory = []CategoryCount{}
	}
	return m
}

// ComputeStats derives the coverage numbers from a result.
func ComputeStats(
	r *Result,
) Stats {
	s := Stats{
		ByResponseKind: map[string]int{},
		TypesByKind:    map[string]int{},
		Endpoints:      len(r.Endpoints),
		Types:          len(r.Decls),
		Modules:        len(r.Modules()),
		Unresolved:     len(r.Unresolved),
	}
	for _, u := range r.Unresolved {
		if u.Severity == SeverityWarning {
			s.UnresolvedWarnings++
			continue
		}
		s.UnresolvedErrors++
	}
	paths := map[string]bool{}
	for _, e := range r.Endpoints {
		paths[e.Path] = true
		s.ByResponseKind[string(e.ResponseKind)]++
		if e.Handler != "" {
			s.HandlersResolved++
		}
		if e.FullyResolved() {
			s.FullyResolved++
		}
		if hasCategory(e.Unresolved, "request-type", "bind-target", "multi-request") || e.Request != nil {
			s.WithRequestBody++
			if e.Request != nil && !hasCategory(e.Unresolved, "request-type", "bind-target", "multi-request") {
				s.RequestResolved++
			}
		}
		if e.ResponseKind == RespJSON {
			s.WithResponsePayload++
			if len(e.Responses) > 0 &&
				!hasCategory(e.Unresolved, "payload-type", "untyped-payload") {
				s.ResponseResolved++
			}
		}
	}
	s.DistinctPaths = len(paths)
	for _, d := range r.Decls {
		s.TypesByKind[string(d.Kind)]++
		if len(d.Dropped) > 0 {
			s.TypesIncomplete++
		}
	}
	return s
}

// hasCategory reports whether any unresolved item falls in one of the
// categories.
func hasCategory(
	list []Unresolved,
	cats ...string,
) bool {
	for _, u := range list {
		for _, c := range cats {
			if u.Category == c {
				return true
			}
		}
	}
	return false
}

// MarshalManifest renders a manifest as stable, indented JSON with a trailing
// newline. Every slice inside it is already sorted, so two runs over the same
// tree produce byte-identical bytes.
func MarshalManifest(
	m Manifest,
) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	return buf.Bytes(), nil
}
