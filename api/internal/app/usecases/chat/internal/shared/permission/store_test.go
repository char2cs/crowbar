package permission_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/permission"
)

func TestStore_GetDefaultsToGuardedForAnUnseenChat(t *testing.T) {
	s := permission.New()
	assert.Equal(t, permission.Guarded, s.Get("never-set"))
}

func TestStore_SetThenGetRoundTrips(t *testing.T) {
	s := permission.New()
	s.Set("chat-1", permission.FullAuto)
	assert.Equal(t, permission.FullAuto, s.Get("chat-1"))
}

func TestStore_ForgetReturnsToTheDefault(t *testing.T) {
	s := permission.New()
	s.Set("chat-1", permission.Trusted)
	s.Forget("chat-1")
	assert.Equal(t, permission.Guarded, s.Get("chat-1"))
}

func TestStore_GetOrDefaultUsesTheFallbackForAnUnseenChat(t *testing.T) {
	s := permission.New()
	assert.Equal(t, permission.FullAuto, s.GetOrDefault("never-set", permission.FullAuto))
}

func TestStore_GetOrDefaultPrefersAnExplicitlySetLevelOverTheFallback(t *testing.T) {
	s := permission.New()
	s.Set("chat-1", permission.Guarded)
	assert.Equal(t, permission.Guarded, s.GetOrDefault("chat-1", permission.FullAuto))
}
