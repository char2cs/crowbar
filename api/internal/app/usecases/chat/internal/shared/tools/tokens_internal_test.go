package tools

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const plantedSecret = "SUPER-SECRET-HMAC-KEY-DO-NOT-LOG"

func plantedMinter(
	t *testing.T,
) *TokenMinter {
	t.Helper()
	m, err := NewTokenMinter()
	require.NoError(t, err)
	m.secret = []byte(plantedSecret)
	return m
}

func TestTokenMinter_NeverPrintsItsSecret(
	t *testing.T,
) {
	m := plantedMinter(t)

	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		ptr := fmt.Sprintf(verb, m)
		assert.NotContains(t, ptr, plantedSecret, "verb %s leaked the secret through a pointer", verb)
		assert.Contains(t, ptr, "REDACTED", "verb %s should render the redaction", verb)

		val := fmt.Sprintf(verb, *m)
		assert.NotContains(t, val, plantedSecret, "verb %s leaked the secret through a copy", verb)
		assert.Contains(t, val, "REDACTED", "verb %s should redact a copy too", verb)
	}
}

func TestTokenMinter_RedactedWhenNestedInAnotherStruct(
	t *testing.T,
) {
	holder := struct {
		name   string
		minter *TokenMinter
	}{name: "usecase", minter: plantedMinter(t)}

	for _, verb := range []string{"%v", "%+v", "%#v"} {
		out := fmt.Sprintf(verb, holder)
		assert.NotContains(t, out, plantedSecret, "verb %s leaked the nested secret", verb)
	}
}

func TestTokenMinter_RedactedInAnExportedField(
	t *testing.T,
) {
	holder := struct {
		Name   string
		Minter *TokenMinter
	}{Name: "usecase", Minter: plantedMinter(t)}

	for _, verb := range []string{"%v", "%+v", "%#v"} {
		out := fmt.Sprintf(verb, holder)
		assert.NotContains(t, out, plantedSecret, "verb %s leaked the nested secret", verb)
		assert.Contains(t, out, "REDACTED", "verb %s should render the redaction", verb)
	}
}
