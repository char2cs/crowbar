package verbs

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

// passArg appends argv. A `positional:` argument becomes exactly one element,
// which is what keeps a user's prompt — provider syntax, spaces, quotes and all —
// data rather than structure.
//
// `value:` is emitted only when the key is PRESENT, not when it is non-empty: a
// flag whose value is legitimately the empty string still needs its own argv slot,
// or the next token silently becomes its value.
func passArg(args Args, plan *models.SpawnPlan) error {
	if args.Present("positional") {
		plan.Argv = append(plan.Argv, args.String("positional"))
		return nil
	}
	plan.Argv = append(plan.Argv, args.String("arg"))
	if args.Present("value") {
		plan.Argv = append(plan.Argv, args.String("value"))
	}
	return nil
}
