package mocks

import (
	"context"

	provider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// ProviderEngine is a fake of the provider operations the import usecase consumes.
type ProviderEngine struct {
	Protected    []string
	ProtectedErr error
	AvatarURL    string
	AvatarURLErr error
	PRLinks      []provider.PRLink
	PRLinksErr   error
}

// NewProviderEngine returns an empty ProviderEngine.
func NewProviderEngine() *ProviderEngine {
	return &ProviderEngine{}
}

func (p *ProviderEngine) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	if p.ProtectedErr != nil {
		return nil, p.ProtectedErr
	}
	return p.Protected, nil
}

func (p *ProviderEngine) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	return p.AvatarURL, p.AvatarURLErr
}

func (p *ProviderEngine) OpenPullRequests(
	ctx context.Context,
	repoPath string,
) ([]provider.PRLink, error) {
	if p.PRLinksErr != nil {
		return nil, p.PRLinksErr
	}
	return p.PRLinks, nil
}
