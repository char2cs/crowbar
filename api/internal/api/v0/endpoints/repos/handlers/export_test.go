package handlers

func (h *Handlers) WithAvatarFetcher(
	fetch AvatarBytesFetcher,
) *Handlers {
	if fetch != nil {
		h.fetchAvatar = fetch
	}
	return h
}
