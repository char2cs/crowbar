package chat

import "context"

// ModelManifestFetchEnabled reports whether a model.manifest: source's
// background refresh may hit the network.
func (u *Usecase) ModelManifestFetchEnabled(
	ctx context.Context,
) (bool, error) {
	return u.manifestFetch.Get(ctx)
}

// SetModelManifestFetchEnabled overwrites the global toggle.
func (u *Usecase) SetModelManifestFetchEnabled(
	ctx context.Context,
	enabled bool,
) error {
	return u.manifestFetch.Set(ctx, enabled)
}
