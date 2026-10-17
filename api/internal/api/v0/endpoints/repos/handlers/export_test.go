package handlers

// WithAvatarFetcher swaps the GitHub avatar fetcher; a nil arg keeps the real one.
func (h *Handlers) WithAvatarFetcher(
	fetch AvatarBytesFetcher,
) *Handlers {
	if fetch != nil {
		h.fetchAvatar = fetch
	}
	return h
}
