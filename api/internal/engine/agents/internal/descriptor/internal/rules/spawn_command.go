package rules

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

type spawnCommand struct{}

func (spawnCommand) Name() string { return "spawn_command" }

// Check enforces the engine's hardest constraint: Crowbar hosts the ordinary
// interactive CLI in a real PTY, and never a headless one. A descriptor that does
// not assert that is rejected outright rather than trusted to behave.
func (spawnCommand) Check(d *spec.Descriptor) error {
	if d.Spawn.Cmd == "" {
		return invalid(d.ID, "missing spawn.cmd")
	}
	if !d.Spawn.InteractiveRequired {
		return invalid(d.ID, "must set spawn.interactive_required")
	}
	return nil
}
