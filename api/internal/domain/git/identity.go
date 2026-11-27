package git

type Identity struct {
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}
