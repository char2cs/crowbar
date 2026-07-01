package git

// Identity is the current human's git/GitHub identity used to attribute
// review comments and other authored actions.
type Identity struct {
	// Login is the GitHub username. Empty when gh is absent or unauthenticated.
	Login string `json:"login"`
	// DisplayName is the user's human-readable name. Falls back to
	// `git config user.name` when gh is unavailable.
	DisplayName string `json:"displayName"`
	// AvatarURL is the GitHub avatar URL. Empty when gh is absent.
	AvatarURL string `json:"avatarUrl"`
}
