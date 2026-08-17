package verbs

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"

func setEnv(args Args, plan *models.SpawnPlan) error {
	plan.Env = append(plan.Env, args.String("name")+"="+args.String("value"))
	return nil
}
