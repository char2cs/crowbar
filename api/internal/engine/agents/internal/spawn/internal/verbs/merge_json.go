package verbs

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
)

// mergeJSON shallow-merges args["set"] into the JSON object at args["path"],
// preserving every other key. It never creates the file: a descriptor that
// wants to ensure a flag in a provider's own persistent config (as opposed to
// a Crowbar-owned ephemeral file, which write_file already covers) has no
// business seeding one from scratch — a missing file means that provider's
// own first-run flow hasn't happened yet, and this step has nothing to fix.
func mergeJSON(args Args, _ *models.SpawnPlan) error {
	path := expandHome(args.String("path"))
	set, _ := args.raw["set"].(map[string]any)
	if len(set) == 0 {
		return nil
	}

	raw, err := os.ReadFile(path) //nolint:gosec // descriptor-declared path, daemon-owned
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("agents: merge_json read: %w", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("agents: merge_json parse: %w", err)
	}

	changed := false
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
	info, err := os.Stat(path)
	mode := os.FileMode(0o600)
	if err == nil {
		mode = info.Mode()
	}
	return os.WriteFile(path, out, mode)
}
