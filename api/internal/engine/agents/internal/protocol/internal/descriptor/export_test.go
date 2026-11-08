package descriptor

import "testing"

// RegisterReader adds a schema version for the life of one test.
func RegisterReader(t *testing.T, version int, r Reader) {
	t.Helper()
	readers[version] = r
	t.Cleanup(func() { delete(readers, version) })
}
