package descriptorcheck

import (
	"regexp"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
)

// loadRule places a daemon load rule in the document and says how to fix it.
type loadRule struct {
	path string
	hint string
}

var loadRules = map[string]loadRule{
	"parse":                 {"events", "the event table must match Crowbar's vocabulary; see the message for the event and field"},
	"identity":              {"id", "set a non-empty id matching the file name"},
	"spawn_command":         {"spawn", "set spawn.cmd and spawn.interactive_required: true"},
	"prompt_submit":         {"presentation.prompt_submit", "prompt_submit needs strategy restart_tui and a session.resume"},
	"catalog_bounds":        {"presentation.slash_catalog", "keep the slash catalog's limits within Crowbar's bounds"},
	"catalog_command":       {"presentation.slash_catalog", "declare a runnable catalog command"},
	"catalog_item_mapping":  {"presentation.slash_catalog", "map every catalog item field Crowbar reads"},
	"catalog_adapter":       {"presentation.slash_catalog", "name a catalog adapter Crowbar ships"},
	"telemetry":             {"telemetry", "declare a telemetry probe Crowbar can run"},
	"selection_strategy":    {"model", "declare how a model/effort selection reaches the CLI"},
	"selection_apply":       {"model", "each selection strategy needs its apply steps"},
	"selection_api_carrier": {"model", "an api-transport provider needs api_apply for its selection"},
	"model_catalog":         {"model", "declare the models the picker offers"},
	"model_discover":        {"model.discover", "a discover source needs a command and a parse rule"},
	"model_manifest":        {"model.discover", "a manifest source needs items_path"},
	"effort_catalog":        {"effort", "declare the efforts each model offers"},
	"terminal_prompts":      {"terminal_prompts", "each terminal prompt needs a needle (and a known kind, if any)"},
	"terminal_notices":      {"terminal_notices", "each terminal notice needs a needle and a known kind"},
	"injected_prompts":      {"injected_prompts", "each injected prompt needs a valid match"},
	"surfaces":              {"surfaces", "surfaces must name a channel the runtime declares"},
	"event_surfaces":        {"events", "an event's surfaces must be ones the descriptor declares"},
}

var eventNameRE = regexp.MustCompile(`event "([^"]+)"`)

func loadFinding(doc document, f protocol.DescriptorRuleFailure) Finding {
	rule, ok := loadRules[f.Rule]
	if !ok {
		rule = loadRule{hint: "see the message"}
	}
	path := rule.path
	if m := eventNameRE.FindStringSubmatch(f.Err.Error()); m != nil {
		path = "events." + m[1]
	}
	return Finding{
		Rule: "load." + f.Rule, Severity: SeverityError, Path: path, Line: doc.line(path),
		Message: f.Err.Error(), Hint: rule.hint,
	}
}
