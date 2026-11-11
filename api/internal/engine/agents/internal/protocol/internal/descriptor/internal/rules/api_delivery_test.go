package rules_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor/internal/rules"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

func TestAPIDelivery_AcceptsAServeOverStdioInNDJSON(t *testing.T) {
	require.NoError(t, rules.Apply(withAPITransport(valid())))
}

// A serve Crowbar cannot speak to is a load failure, not a spawn that waits
// on a channel nobody opened.
func TestAPIDelivery_RejectsAServeThatDoesNotSayHowItIsSpokenTo(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*spec.APISpec)
		wantMsg string
	}{
		"no delivery":       {func(a *spec.APISpec) { a.Delivery = "" }, "runtime.api.delivery"},
		"a socket delivery": {func(a *spec.APISpec) { a.Delivery = "unix" }, "runtime.api.delivery"},
		"no format":         {func(a *spec.APISpec) { a.Format = "" }, "runtime.api.format"},
		"a framed format":   {func(a *spec.APISpec) { a.Format = "content-length" }, "runtime.api.format"},
	} {
		t.Run(name, func(t *testing.T) {
			d := withAPITransport(valid())
			tc.mutate(&d.Runtime.API)

			err := rules.Apply(d)

			require.ErrorIs(t, err, rules.ErrInvalidDescriptor)
			assert.Contains(t, err.Error(), tc.wantMsg)
		})
	}
}

func TestAPIDelivery_ADescriptorWithNoServeDeclaresNone(t *testing.T) {
	d := valid()
	d.Runtime.API = spec.APISpec{Protocol: "jsonrpc2", Attach: []string{"probe", "resume", "{session_id}"}}

	require.NoError(t, rules.Apply(d))
}
