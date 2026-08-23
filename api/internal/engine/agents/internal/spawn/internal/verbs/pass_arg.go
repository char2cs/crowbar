package verbs

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

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
