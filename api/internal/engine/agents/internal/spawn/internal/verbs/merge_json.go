package verbs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
)

// mergeJSON ensures args["set"] is present in the JSON object at args["path"],
// preserving every other key. With no args["seed_from"], it never creates the
// file: a descriptor that wants to ensure a flag in a provider's own
// persistent config (as opposed to a Crowbar-owned ephemeral file, which
// write_file already covers) has no business seeding one from scratch — a
// missing file means that provider's own first-run flow genuinely hasn't
// happened, and this step has nothing to fix.
//
// With args["seed_from"], a missing destination is instead seeded by copying
// that source file's JSON — with args["clear"] keys reset to {} in the copy,
// e.g. a provider's own per-directory trust/project history, which belongs to
// wherever the real, unmanaged install is really running, not to this
// isolated copy — before applying set. An existing destination is only ever
// merged, never re-seeded: seeding is a one-time bootstrap, not something
// that should silently overwrite whatever has accumulated in the isolated
// copy since (Crowbar's own history of using it, for instance).
func mergeJSON(args Args, _ *models.SpawnPlan) error {
	path := expandHome(args.String("path"))
	set, _ := args.raw["set"].(map[string]any)

	doc, seeded, err := readOrSeedJSON(path, args)
	if err != nil {
		return err
	}
	if doc == nil {
		return nil // no file, no seed source — nothing to fix
	}

	changed := seeded
	for k, v := range set {
		if existing, ok := doc[k]; !ok || existing != v {
			doc[k] = v
			changed = true
		}
	}
	if !changed {
		return nil
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("agents: merge_json marshal: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode()
	} else if seeded {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("agents: merge_json seed mkdir: %w", err)
		}
	}
	return os.WriteFile(path, out, mode)
}

// readOrSeedJSON reads path's JSON object. If path doesn't exist and
// args["seed_from"] is set, it seeds doc from that source instead (applying
// args["clear"]) and reports seeded=true so the caller knows to write even
// with no `set` keys actually changing anything.
func readOrSeedJSON(path string, args Args) (doc map[string]any, seeded bool, err error) {
	raw, readErr := os.ReadFile(path) //nolint:gosec // descriptor-declared path, daemon-owned
	if readErr == nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, false, fmt.Errorf("agents: merge_json parse: %w", err)
		}
		return doc, false, nil
	}
	if !os.IsNotExist(readErr) {
		return nil, false, fmt.Errorf("agents: merge_json read: %w", readErr)
	}

	seedFrom := args.String("seed_from")
	if seedFrom == "" {
		return nil, false, nil
	}
	seedRaw, err := os.ReadFile(expandHome(seedFrom)) //nolint:gosec // descriptor-declared path, daemon-owned
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("agents: merge_json seed read: %w", err)
	}
	doc = map[string]any{}
	if err := json.Unmarshal(seedRaw, &doc); err != nil {
		return nil, false, fmt.Errorf("agents: merge_json seed parse: %w", err)
	}
	if clearKeys, ok := args.raw["clear"].([]any); ok {
		for _, k := range clearKeys {
			if key, ok := k.(string); ok {
				doc[key] = map[string]any{}
			}
		}
	}
	return doc, true, nil
}
