//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type identityDTO struct {
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

func TestIdentity_CurrentIdentity(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var id identityDTO
	h.get(base+"/identity", &id)

	assert.NotEmpty(t, id.DisplayName, "displayName must be non-empty (git config user.name fallback sets it to 't')")
}
