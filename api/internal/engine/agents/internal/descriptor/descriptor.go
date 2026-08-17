// Package descriptor loads and validates provider descriptors.
//
// A descriptor is resolved from one of two places: the set embedded in the
// binary, or an override on disk under the Crowbar home. The override always
// wins, which is what makes a provider's behaviour adjustable without a release —
// and why every descriptor, embedded or not, goes through the same validation.
package descriptor

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

//go:embed descriptors/*.yaml
var embedded embed.FS

const (
	embeddedDir = "descriptors"
	overrideDir = "descriptors"
	yamlSuffix  = ".yaml"
)

// ErrUnknown reports a provider id no descriptor answers to.
var ErrUnknown = fmt.Errorf("agents: unknown provider")

// ErrInvalid is re-exported from the rules package so callers match one sentinel.
var ErrInvalid = rules.ErrInvalidDescriptor

// Load parses and validates descriptor bytes.
func Load(data []byte) (*spec.Descriptor, error) {
	var d spec.Descriptor
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("agents: descriptor unmarshal: %w", err)
	}
	if err := rules.Apply(&d); err != nil {
		return nil, err
	}
	return &d, nil
}

// Resolve loads one descriptor by id: a disk override at
// <homeDir>/descriptors/<id>.yaml wins, else the embedded default.
//
// homeDir is untrusted only in the sense that it is caller-supplied; id is not
// joined into a path without being checked first, because a provider id arrives
// from a runner record and a traversal there would read an arbitrary file.
func Resolve(ctx context.Context, homeDir, id string) (*spec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validID(id) {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	if homeDir != "" {
		override := filepath.Join(homeDir, overrideDir, id+yamlSuffix)
		if data, err := os.ReadFile(override); err == nil { //nolint:gosec // id is validated above; homeDir is daemon-owned
			return Load(data)
		}
	}
	data, err := embedded.ReadFile(embeddedDir + "/" + id + yamlSuffix)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return Load(data)
}

// All enumerates every known descriptor: the embedded ids plus any id present on
// disk under homeDir, deduped with the override winning, sorted by id so the feed
// is deterministic.
//
// A single unreadable or invalid descriptor does NOT fail the enumeration. This
// backs the provider picker, and one broken user override should cost that
// provider its row, not the whole list — the alternative is a UI that goes blank
// and says nothing about why.
func All(ctx context.Context, homeDir string) ([]*spec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := embedded.ReadDir(embeddedDir)
	if err != nil {
		return nil, fmt.Errorf("agents: list embedded descriptors: %w", err)
	}
	ids := idSet(entries)
	if homeDir != "" {
		// A missing overrides dir is not an error: it means no overrides.
		diskEntries, _ := os.ReadDir(filepath.Join(homeDir, overrideDir))
		for id := range idSet(diskEntries) {
			ids[id] = struct{}{}
		}
	}

	out := make([]*spec.Descriptor, 0, len(ids))
	for id := range ids {
		d, err := Resolve(ctx, homeDir, id)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Installed reports whether a provider's CLI is present: its spawn command
// resolves to an executable file.
//
// Install-only, and deliberately so: neither CLI exposes a reliable
// machine-readable auth check, so a logged-out-but-installed CLI reads installed
// and fails at spawn, where the failure can be explained.
func Installed(cmd string) bool {
	if cmd == "" {
		return false
	}
	info, err := os.Stat(binpath.Resolve(cmd))
	return err == nil && !info.IsDir()
}

// validID rejects anything that is not a bare file stem. Descriptor ids reach
// this package from persisted runner records and from HTTP, and both are joined
// into a filesystem path.
func validID(id string) bool {
	if id == "" || strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return false
	}
	return id == filepath.Base(id)
}

func idSet(entries []fs.DirEntry) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), yamlSuffix) {
			continue
		}
		if id := strings.TrimSuffix(e.Name(), yamlSuffix); validID(id) {
			ids[id] = struct{}{}
		}
	}
	return ids
}
