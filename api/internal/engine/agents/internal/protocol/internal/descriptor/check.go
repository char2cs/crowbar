package descriptor

import (
	"fmt"
	"os"
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type RuleFailure struct {
	Rule string
	Err  error
}

func Check(raw []byte) (*spec.Descriptor, []RuleFailure) {
	d, err := Parse(raw)
	if err != nil {
		return nil, []RuleFailure{{Rule: "parse", Err: err}}
	}
	var out []RuleFailure
	for _, rule := range rules.All() {
		if err := rule.Check(d); err != nil {
			out = append(out, RuleFailure{Rule: rule.Name(), Err: err})
		}
	}
	return d, out
}

type Source struct {
	ID   string
	Path string
	Raw  []byte
}

func Sources(homeDir string) ([]Source, error) {
	entries, err := embedded.ReadDir(embeddedDir)
	if err != nil {
		return nil, fmt.Errorf("agents: list embedded descriptors: %w", err)
	}
	ids := idSet(entries)
	if homeDir != "" {
		for id := range idSet(overrideEntries(homeDir)) {
			ids[id] = struct{}{}
		}
	}
	out := make([]Source, 0, len(ids))
	for id := range ids {
		if src, ok := source(homeDir, id); ok {
			out = append(out, src)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func SourceFor(homeDir, id string) (Source, bool) {
	if !validID(id) {
		return Source{}, false
	}
	return source(homeDir, id)
}

func source(homeDir, id string) (Source, bool) {
	if override := OverridePath(homeDir, id); override != "" {
		if raw, err := os.ReadFile(override); err == nil { //nolint:gosec // id passed validID; homeDir is daemon-owned
			return Source{ID: id, Path: override, Raw: raw}, true
		}
	}
	return EmbeddedSource(id)
}

func EmbeddedSource(id string) (Source, bool) {
	if !validID(id) {
		return Source{}, false
	}
	raw, err := embedded.ReadFile(embeddedDir + "/" + id + yamlSuffix)
	if err != nil {
		return Source{}, false
	}
	return Source{ID: id, Raw: raw}, true
}
