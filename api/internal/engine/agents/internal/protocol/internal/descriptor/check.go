package descriptor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// RuleFailure is one load rule a descriptor breaks. Rule is "parse" when the
// document never reached the rules (YAML, the event vocabulary, a channel split).
type RuleFailure struct {
	Rule string
	Err  error
}

// Check is Load that reports every failing rule instead of stopping at the
// first, for a validator that shows the whole picture at once.
func Check(raw []byte) (*spec.Descriptor, []RuleFailure) {
	d, err := ParseV3(raw)
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

// Source is one descriptor document Crowbar would load: the embedded default,
// or the on-disk override that replaces it.
type Source struct {
	ID string
	// Path is the override file, or "" for the embedded default.
	Path string
	Raw  []byte
}

// Sources returns every descriptor document, sorted by id, with an override
// shadowing the embedded default of the same id exactly as Resolve does.
func Sources(homeDir string) ([]Source, error) {
	entries, err := embedded.ReadDir(embeddedDir)
	if err != nil {
		return nil, fmt.Errorf("agents: list embedded descriptors: %w", err)
	}
	ids := idSet(entries)
	if homeDir != "" {
		diskEntries, _ := os.ReadDir(filepath.Join(homeDir, overrideDir))
		for id := range idSet(diskEntries) {
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

// SourceFor is the one document Crowbar would load for id: its override, else
// the shipped default.
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
	raw, err := embedded.ReadFile(embeddedDir + "/" + id + yamlSuffix)
	if err != nil {
		return Source{}, false
	}
	return Source{ID: id, Raw: raw}, true
}
