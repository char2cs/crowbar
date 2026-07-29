package agenttools

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenMinter_NeverPrintsItsSecret covers every verb a log line or a panic
// dump might reach the minter through. The secret is planted rather than random
// so the assertion is about the FORMATTING, not about whether random bytes
// happened to be printable.
func TestTokenMinter_NeverPrintsItsSecret(
	t *testing.T,
) {
	const secret = "SUPER-SECRET-HMAC-KEY-DO-NOT-LOG"
	m, err := NewTokenMinter()
	require.NoError(t, err)
	m.secret = []byte(secret)

	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(verb, m)
		assert.NotContains(t, out, secret, "verb %s leaked the secret", verb)
		assert.Contains(t, out, "REDACTED", "verb %s should render the redaction", verb)
	}
}

// TestTokenMinter_RedactedWhenNestedInAnotherStruct is the case that actually
// matters: the minter is a FIELD of the agent usecase, so the realistic leak is
// someone printing the struct that holds it, not the minter itself.
func TestTokenMinter_RedactedWhenNestedInAnotherStruct(
	t *testing.T,
) {
	const secret = "SUPER-SECRET-HMAC-KEY-DO-NOT-LOG"
	m, err := NewTokenMinter()
	require.NoError(t, err)
	m.secret = []byte(secret)

	holder := struct {
		Name   string
		Minter *TokenMinter
	}{Name: "usecase", Minter: m}

	for _, verb := range []string{"%v", "%+v"} {
		out := fmt.Sprintf(verb, holder)
		assert.NotContains(t, out, secret, "verb %s leaked the nested secret", verb)
	}
}
