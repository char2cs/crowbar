//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// identityDTO mirrors the wire shape of GET .../identity.
type identityDTO struct {
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

// TestIdentity_CurrentIdentity proves that GET .../identity always returns 200
// with a non-empty displayName. The test harness repo sets `git config
// user.name "t"`, so the git-config fallback guarantees a non-empty result even
// when gh is absent.
func TestIdentity_CurrentIdentity(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var id identityDTO
	h.get(base+"/identity", &id)

	assert.NotEmpty(t, id.DisplayName, "displayName must be non-empty (git config user.name fallback sets it to 't')")
}
