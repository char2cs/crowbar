package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type spawnCommand struct{}

func (spawnCommand) Name() string { return "spawn_command" }

func (spawnCommand) Check(d *spec.Descriptor) error {
	if d.Spawn.Cmd == "" {
		return invalid(d.ID, "missing spawn.cmd")
	}
	if !d.Spawn.InteractiveRequired {
		return invalid(d.ID, "must set spawn.interactive_required")
	}
	return nil
}
