package tools_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
)

func TestTokenMinter_VerifiesItsOwnToken(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)

	tok := m.Mint("runner-a")
	require.NotEmpty(t, tok)
	require.True(t, m.Verify("runner-a", tok))
}

func TestTokenMinter_RejectsTokenForAnotherRunner(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)

	// The whole point: holding runner A's token must not grant runner B's scope.
	require.False(t, m.Verify("runner-b", m.Mint("runner-a")))
}

func TestTokenMinter_RejectsGarbage(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)

	require.False(t, m.Verify("runner-a", ""))
	require.False(t, m.Verify("runner-a", "not-base64-$$$"))
	require.False(t, m.Verify("", m.Mint("")))
}

// Two minters model two daemon boots. Runners never survive a boot, so a token
// from the previous one must be dead on arrival.
func TestTokenMinter_TokensDoNotSurviveAReboot(t *testing.T) {
	first, err := tools.NewTokenMinter()
	require.NoError(t, err)
	second, err := tools.NewTokenMinter()
	require.NoError(t, err)

	require.False(t, second.Verify("runner-a", first.Mint("runner-a")))
}

// The token travels in argv and in a JSON/TOML config value, so it must be safe
// to embed in both without escaping.
func TestTokenMinter_TokenIsURLSafeBase64(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)

	tok := m.Mint("runner-a")
	require.NotContains(t, tok, "+")
	require.NotContains(t, tok, "/")
	require.NotContains(t, tok, "=")
	require.NotContains(t, tok, `"`)
}
