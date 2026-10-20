package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// apiDelivery requires a serve to say how Crowbar speaks to it. Crowbar opens
// serve's stdio before the fork, so its first answer is the only readiness.
type apiDelivery struct{}

func (apiDelivery) Name() string { return "api_delivery" }

func (apiDelivery) Check(d *spec.Descriptor) error {
	api := d.Runtime.API
	if len(api.Serve) == 0 {
		return nil
	}
	if api.Delivery != spec.APIDeliveryStdio {
		return invalid(d.ID, "runtime.api.delivery must be %q, got %q", spec.APIDeliveryStdio, api.Delivery)
	}
	if api.Format != spec.APIFormatNDJSON {
		return invalid(d.ID, "runtime.api.format must be %q, got %q", spec.APIFormatNDJSON, api.Format)
	}
	return nil
}
