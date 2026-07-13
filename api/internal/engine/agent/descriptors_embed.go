package agent

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed descriptors/*.yaml
var embedded embed.FS

// ResolveDescriptor loads provider descriptor by id: a disk override at
// <homeDir>/descriptors/<id>.yaml wins, else the embedded default.
func ResolveDescriptor(homeDir, providerID string) (*Descriptor, error) {
	override := filepath.Join(homeDir, "descriptors", providerID+".yaml")
	if data, err := os.ReadFile(override); err == nil {
		return LoadDescriptor(data)
	}
	data, err := embedded.ReadFile("descriptors/" + providerID + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("agent: unknown provider %q: %w", providerID, err)
	}
	return LoadDescriptor(data)
}

// AllDescriptors enumerates every known provider descriptor: the ids embedded
// under descriptors/*.yaml plus any id present on disk at
// <homeDir>/descriptors/*.yaml (future user-managed providers), id-deduped
// with the on-disk override winning per id (each id is loaded through
// ResolveDescriptor, which already prefers the disk override), sorted by id
// for a deterministic feed. It backs GET .../agent/providers (the lazy-by-id
// ResolveDescriptor cannot list).
func AllDescriptors(homeDir string) ([]*Descriptor, error) {
	embeddedEntries, err := embedded.ReadDir("descriptors")
	if err != nil {
		return nil, fmt.Errorf("agent: list embedded descriptors: %w", err)
	}
	ids := descriptorIDSet(embeddedEntries)

	// A missing on-disk overrides dir is not an error: it just means no
	// overrides or additions on top of the embedded set.
	diskEntries, _ := os.ReadDir(filepath.Join(homeDir, "descriptors"))
	for id := range descriptorIDSet(diskEntries) {
		ids[id] = struct{}{}
	}

	out := make([]*Descriptor, 0, len(ids))
	for id := range ids {
		d, err := ResolveDescriptor(homeDir, id)
		if err != nil {
			return nil, fmt.Errorf("agent: enumerate descriptor %q: %w", id, err)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func descriptorIDSet(entries []fs.DirEntry) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if id, ok := descriptorID(e.Name()); ok {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func descriptorID(name string) (string, bool) {
	if !strings.HasSuffix(name, ".yaml") {
		return "", false
	}
	return strings.TrimSuffix(name, ".yaml"), true
}
