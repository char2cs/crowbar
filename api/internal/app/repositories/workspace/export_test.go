package workspace

// CachedEntityCount reports the number of entities currently held in the
// per-entity LRU registry. Test-only: it lets external tests assert the cache is
// bounded by the configured maxOpen instead of growing without bound. It type
// asserts to the concrete *workspace so it stays out of the public interface.
func CachedEntityCount(
	repo Workspace,
) int {
	w, ok := repo.(*workspace)
	if !ok {
		return -1
	}
	return w.entities.Len()
}
