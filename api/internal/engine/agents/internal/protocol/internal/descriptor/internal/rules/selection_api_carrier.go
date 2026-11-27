package rules

import (
	"slices"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type selectionAPICarrier struct{}

func (selectionAPICarrier) Name() string { return "selection_api_carrier" }

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
