package runner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type fakeSelectionAgent struct {
	engineagents.Agent
	models     []string
	efforts    []string
	discovered bool
}

func (f fakeSelectionAgent) ID() string                 { return "fake-provider" }
func (f fakeSelectionAgent) Models() []string           { return f.models }
func (f fakeSelectionAgent) Efforts(string) []string    { return f.efforts }
func (f fakeSelectionAgent) PermissionLevels() []string { return nil }
func (f fakeSelectionAgent) Capabilities() engineagents.Capabilities {
	return engineagents.Capabilities{ModelDiscovery: f.discovered}
}

func TestResolveSelectionForSpawn_KeepsAModelTheProviderDeclares(t *testing.T) {
	a := fakeSelectionAgent{models: []string{"opus", "sonnet"}, efforts: []string{"low", "high"}}

	got := resolveSelectionForSpawn(a, engineagents.Selection{Model: "opus", Effort: "high"})

	assert.Equal(t, "opus", got.Model)
	assert.Equal(t, "high", got.Effort)
}

func TestResolveSelectionForSpawn_ClearsAModelAStaticCatalogueDoesNotDeclare(t *testing.T) {
	a := fakeSelectionAgent{models: []string{"opus", "sonnet"}, discovered: false}

	got := resolveSelectionForSpawn(a, engineagents.Selection{Model: "not-a-real-model", Effort: "low"})

	assert.Empty(t, got.Model, "a non-empty, resolved catalogue that omits the model must clear it")
	assert.Empty(t, got.Effort, "efforts are read against the cleared model, so they clear too")
}

func TestResolveSelectionForSpawn_KeepsAnUnresolvedDiscoveredModelUnchanged(t *testing.T) {
	a := fakeSelectionAgent{discovered: true}

	got := resolveSelectionForSpawn(a, engineagents.Selection{Model: "gpt-6-astra", Effort: "ultra"})

	assert.Equal(t, "gpt-6-astra", got.Model)
	assert.Equal(t, "ultra", got.Effort)
}
