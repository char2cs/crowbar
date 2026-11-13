package rules

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type selectionAPICarrier struct{}

func (selectionAPICarrier) Name() string { return "selection_api_carrier" }

// Check refuses an api-transport descriptor whose model/effort is declared only on
// the argv carrier: such a spawn forks no process, so apply: would be dropped while
// the runner records it as honoured. api_apply: carries it over the connection.
func (selectionAPICarrier) Check(d *spec.Descriptor) error {
	if d.Runtime.Transport != "api" {
		return nil
	}
	if dialed(d) {
		return dialedCarrier(d)
	}
	if d.Model != nil && len(d.Model.APIApply) == 0 {
		return invalid(d.ID, "model.api_apply is empty: an api-transport spawn can fork no process for model.apply to ride")
	}
	if d.Effort != nil && len(d.Effort.APIApply) == 0 {
		return invalid(d.ID, "effort.api_apply is empty: an api-transport spawn can fork no process for effort.apply to ride")
	}
	return nil
}

// dialed is a provider reached over its own socket or url, which forks no process
// at all, so its choices can only ride the call that opens the session.
func dialed(d *spec.Descriptor) bool {
	if d.Runtime.Transport != "api" {
		return false
	}
	delivery := d.Runtime.API.Delivery
	return delivery == spec.APIDeliveryWebsocket || delivery == spec.APIDeliveryHTTP
}

func dialedCarrier(d *spec.Descriptor) error {
	carried := func(placeholder string) bool {
		return slices.ContainsFunc(d.Events["prompt"].Fresh, func(step spec.CallStep) bool {
			return mentions(step.Send, placeholder)
		})
	}
	if d.Model != nil && !carried("{model") {
		return invalid(d.ID, "model is declared but no step sends {model} when it opens the session")
	}
	if d.Effort != nil && !carried("{effort") {
		return invalid(d.ID, "effort is declared but no step sends {effort} when it opens the session")
	}
	return nil
}

func mentions(node any, placeholder string) bool {
	switch v := node.(type) {
	case string:
		return strings.Contains(v, placeholder)
	case map[string]any:
		for _, child := range v {
			if mentions(child, placeholder) {
				return true
			}
		}
	case []any:
		return slices.ContainsFunc(v, func(child any) bool { return mentions(child, placeholder) })
	}
	return false
}
