package descriptor

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/core/svgicon"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

//go:embed descriptors-v3/*.yaml descriptors-v3/*.json
var embedded embed.FS

const (
	// The shipped descriptors are v3 (event-centric).
	embeddedDir = "descriptors-v3"
	overrideDir = "descriptors"
	yamlSuffix  = ".yaml"

	// modelManifestFile is the one bundled model.manifest: fallback shared by every
	// provider; each descriptor's items_path picks its own rows out of it.
	modelManifestFile = embeddedDir + "/model-manifest.json"
)

var ErrUnknown = fmt.Errorf("agents: unknown provider")

var ErrInvalid = rules.ErrInvalidDescriptor

// Load parses a descriptor and validates its event table against Crowbar's canonical
// vocabulary, which is what makes a typo a startup failure rather than a field that
// silently maps to nothing.
func Load(data []byte) (*spec.Descriptor, error) {
	d, err := ParseV3(data)
	if err != nil {
		return nil, err
	}
	// The webview injects the icon as HTML: one that is not plain SVG is dropped,
	// never fatal, and descriptorcheck reports it.
	if d.Icon != "" && svgicon.Validate(d.Icon) != nil {
		d.Icon = ""
	}
	if err := rules.Apply(d); err != nil {
		return nil, err
	}
	return d, nil
}

// Resolve loads id's descriptor: its on-disk override when accept admits it (nil
// admits all), else the shipped default, so a refused override never takes the
// provider down while a default exists.
func Resolve(ctx context.Context, homeDir, id string, accept func(raw []byte) bool) (*spec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validID(id) {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	if override := OverridePath(homeDir, id); override != "" {
		data, err := os.ReadFile(override) //nolint:gosec // id is validated above; homeDir is daemon-owned
		if err == nil && (accept == nil || accept(data) || !hasEmbedded(id)) {
			return Load(data)
		}
	}
	data, err := embedded.ReadFile(embeddedDir + "/" + id + yamlSuffix)
	if err != nil {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	return Load(data)
}

// OverridePath is where Resolve looks for id's on-disk override under homeDir ("" when
// homeDir is). Exposed so a hot-path caller can Stat it instead of re-resolving.
func OverridePath(homeDir, id string) string {
	if homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, overrideDir, id+yamlSuffix)
}

func All(ctx context.Context, homeDir string, accept func(raw []byte) bool) ([]*spec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

	out := make([]*spec.Descriptor, 0, len(ids))
	for id := range ids {
		d, err := Resolve(ctx, homeDir, id, accept)
		if err != nil {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// overrideEntries lists homeDir's override directory. One that cannot be read
// leaves the embedded set standing, as a broken override does, and says so.
func overrideEntries(homeDir string) []fs.DirEntry {
	dir := filepath.Join(homeDir, overrideDir)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("agents: list descriptor overrides", "dir", dir, "err", err)
	}
	return entries
}

func hasEmbedded(id string) bool {
	_, err := embedded.ReadFile(embeddedDir + "/" + id + yamlSuffix)
	return err == nil
}

// EmbeddedModelManifest is the bundled model.manifest: fallback, or nil when the
// embed is missing, so a bad bundle degrades instead of failing boot.
func EmbeddedModelManifest() []byte {
	data, err := embedded.ReadFile(modelManifestFile)
	if err != nil {
		return nil
	}
	return data
}

func Installed(cmd string) bool {
	if cmd == "" {
		return false
	}
	info, err := os.Stat(binpath.Resolve(cmd))
	return err == nil && !info.IsDir()
}

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
