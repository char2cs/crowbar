package agenttools

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// plantedSecret is planted rather than random so every assertion below is about
// the FORMATTING, not about whether random bytes happened to be printable.
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

// TestTokenMinter_NeverPrintsItsSecret covers every verb a log line or a panic
// dump might reach the minter through, for both a pointer and a COPY.
//
// The copy is the case worth spelling out: a pointer-receiver String would not
// apply to it, so printing a dereferenced minter would silently fall back to the
// default struct formatter and render the raw key.
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

// TestTokenMinter_RedactedWhenNestedInAnotherStruct models the REAL holder: the
// agent usecase keeps the minter in an UNEXPORTED field, so this fixture does
// too.
//
// fmt cannot call String on an unexported field at all — it prints the bare
// pointer instead — so the redaction string is deliberately NOT expected here.
// What must hold is only that nothing in the struct's rendering reaches the
// secret, which is the property the usecase actually relies on.
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

// TestTokenMinter_RedactedInAnExportedField covers the other holder shape. An
// exported field DOES let fmt call String, and what it calls must be the
// redaction rather than the default struct formatter.
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
