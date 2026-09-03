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

//go:embed descriptors-v3/*.yaml
var embedded embed.FS

const (
	// The shipped descriptors are v3 (event-centric). The v2 tree is retained only
	// so the migration-completeness test can compare against it, and is deleted with
	// the v2 code paths.
	embeddedDir = "descriptors-v3"
	overrideDir = "descriptors"
	yamlSuffix  = ".yaml"
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

func Resolve(ctx context.Context, homeDir, id string) (*spec.Descriptor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validID(id) {
		return nil, fmt.Errorf("%w: %q", ErrUnknown, id)
	}
	if override := OverridePath(homeDir, id); override != "" {
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
