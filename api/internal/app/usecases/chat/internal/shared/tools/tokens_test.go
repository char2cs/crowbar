package tools_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
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

func TestTokenMinter_RelayTokensAreKeyedByTheRelayAndLeaveRunnersAlone(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)

	tok := m.MintRelay("relay-a")
	require.True(t, m.VerifyRelay("relay-a", tok))
	require.False(t, m.VerifyRelay("relay-b", tok), "another binding's id is refused")
	require.False(t, m.Verify("relay-a", tok), "a relay token is no runner's")
	require.False(t, m.VerifyRelay("relay-a", m.Mint("relay-a")), "a runner's token is no relay's")
	require.False(t, m.VerifyRelay("", m.MintRelay("")))
	require.False(t, m.VerifyRelay("relay-a", ""))
	require.False(t, m.VerifyRelay("relay-a", "not-base64-$$$"))

	reboot, err := tools.NewTokenMinter()
	require.NoError(t, err)
	require.False(t, reboot.VerifyRelay("relay-a", tok), "the next boot's secret retires the token the relay holds")
	require.True(t, reboot.VerifyRelay("relay-a", reboot.MintRelay("relay-a")))
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
