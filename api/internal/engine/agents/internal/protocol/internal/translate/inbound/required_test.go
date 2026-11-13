package inbound_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func requiring(canonical string) *spec.Descriptor {
	d := descriptor(map[string]map[string]string{canonical: {"session_id": "session_id", "text": "payload.text"}})
	ev := d.Events[canonical]
	ev.Required = []string{"session_id", "text"}
	d.Events[canonical] = ev
	return d
}

// A delta frame whose text is an explicit null carries nothing to show: it is dropped
// quietly like any other variant this event does not take, not failed as a broken mapping.
func TestParse_ADeltaWithNullTextIsDroppedQuietly(t *testing.T) {
	for _, kind := range []string{spec.HookMessageDelta, spec.HookReasoningDelta, spec.HookToolOutputDelta} {
		_, err := parse(requiring(kind), kind, []byte(`{"session_id":"s1","payload":{"text":null}}`))

		require.ErrorIs(t, err, inbound.ErrVariantMismatch, kind)
		assert.NotErrorIs(t, err, inbound.ErrRequiredFieldMissing, kind)
	}
}

// The rule is narrow: an absent text, a null on any other event, and a null in a
// different required slot all stay hard errors.
func TestParse_OnlyANullDeltaTextIsExcused(t *testing.T) {
	cases := map[string]struct{ kind, payload string }{
		"absent text on a delta":        {spec.HookMessageDelta, `{"session_id":"s1","payload":{}}`},
		"null session on a delta":       {spec.HookMessageDelta, `{"session_id":null,"payload":{"text":"x"}}`},
		"null text on a non-delta kind": {spec.HookTurnStop, `{"session_id":"s1","payload":{"text":null}}`},
	}
	for name, c := range cases {
		_, err := parse(requiring(c.kind), c.kind, []byte(c.payload))

		require.ErrorIs(t, err, inbound.ErrRequiredFieldMissing, name)
	}
}
