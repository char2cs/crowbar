package chat

import "context"

func (u *Usecase) ModelManifestFetchEnabled(
	ctx context.Context,
) (bool, error) {
	return u.manifestFetch.Get(ctx)
}

func (u *Usecase) SetModelManifestFetchEnabled(
	ctx context.Context,
	enabled bool,
) error {
	return u.manifestFetch.Set(ctx, enabled)
}
