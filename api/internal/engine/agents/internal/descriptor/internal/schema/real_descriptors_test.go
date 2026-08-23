package schema_test

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor/internal/schema"
)

type v2Descriptor struct {
	ID    string `yaml:"id"`
	Hooks struct {
		Events map[string]map[string]string `yaml:"events"`
	} `yaml:"hooks"`
}

// The vocabulary must accept the descriptors that ship today, unchanged. If it does
// not, the table is missing a field the providers really use, and migrating them to v3
// would silently drop it.
//
// This runs against the v2 files on purpose: it is the proof that the data table is a
// faithful transcription of the Go rules it replaces, taken BEFORE the descriptors are
// rewritten.
func TestVocabulary_AcceptsTheShippedDescriptors(t *testing.T) {
	v, err := schema.Load()
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join("..", "..", "descriptors")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	var checked int
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var d v2Descriptor
		if err := yaml.Unmarshal(raw, &d); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if len(d.Hooks.Events) == 0 {
			continue // already migrated to v3; the v3 loader's own test covers it
		}
		checked++
		if err := v.Validate(d.ID, d.Hooks.Events); err != nil {
			t.Errorf("%s: %v", e.Name(), err)
		}
	}

	if checked == 0 {
		t.Skip("no v2 descriptors left to check — superseded by the v3 loader test")
	}
}
