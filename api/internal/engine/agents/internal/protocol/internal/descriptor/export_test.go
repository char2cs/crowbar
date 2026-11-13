package descriptor

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// RegisterReader adds a schema version for the life of one test.
func RegisterReader(t *testing.T, version int, r Reader) {
	t.Helper()
	readers[version] = r
	t.Cleanup(func() { delete(readers, version) })
}

// ShippedIDs is the embedded descriptor set as the source tree names it, sorted.
func ShippedIDs(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob("descriptors-v3/*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, strings.TrimSuffix(filepath.Base(m), ".yaml"))
	}
	sort.Strings(ids)
	return ids
}
