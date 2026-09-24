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

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

//go:embed descriptors-v3/*.yaml descriptors-v3/*.json
var embedded embed.FS

const (
	// The shipped descriptors are v3 (event-centric). The v2 tree is retained only
	// so the migration-completeness test can compare against it, and is deleted with
	// the v2 code paths.
	embeddedDir = "descriptors-v3"
	overrideDir = "descriptors"
	yamlSuffix  = ".yaml"

	// modelManifestFile is the one bundled model.manifest: fallback, shared by
	// every provider that declares that source — not provider-scoped, since
	// the descriptor's own items_path is what picks a provider's rows out of
	// it.
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
	if err := rules.Apply(d); err != nil {
		return nil, err
	}
	return d, nil
}

// Resolve loads id's descriptor: its on-disk override when accept admits it
// (a nil accept admits every override), else the shipped default. An override
// accept refuses — one copied from an older shipped descriptor, say — never
// takes the provider down with it while a default exists.
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

// OverridePath is where a per-daemon on-disk override for id would live under
// homeDir, the same path Resolve itself checks before falling back to the
// embedded default. Exposed so a caller that resolves the SAME id repeatedly
// on a hot path (agents.service.Get, called once per ingested hook — see its
// own doc comment) can cheaply Stat this path to know whether Resolve's full
// read-parse-validate is worth repeating, instead of paying it unconditionally
// on every call. Empty for a homeDir-less caller, exactly like Resolve's own
// "no override possible" case.
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
		diskEntries, _ := os.ReadDir(filepath.Join(homeDir, overrideDir))
		for id := range idSet(diskEntries) {
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

func hasEmbedded(id string) bool {
	_, err := embedded.ReadFile(embeddedDir + "/" + id + yamlSuffix)
	return err == nil
}

// EmbeddedModelManifest is the bundled model.manifest: fallback — read fresh
// on every call (it never changes at runtime) rather than cached, so a
// missing/corrupt embed degrades to nil bytes rather than panicking or
// blocking boot; ProbeManifest's own ErrMalformedOutput carries that
// forward.
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
