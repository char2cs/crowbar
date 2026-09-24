package conversation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// stubLiveRunnerForSelection answers only LiveRunnerForChat, so
// ChatProviderID resolves without a real runner-event store — the same
// panic-on-anything-else pattern the runner package's own narrow stubs use.
type stubLiveRunnerForSelection struct {
	agentrunner.EventStore
	runner engineagents.Runner
}

func (s stubLiveRunnerForSelection) LiveRunnerForChat(
	context.Context, string,
) (engineagents.Runner, error) {
	return s.runner, nil
}

// fakeSelectionAgent mirrors the runner package's own fixture of the same
// name — answers only Models/Efforts/Capabilities/ID, everything else
// panics via the embedded nil interface.
type fakeSelectionAgent struct {
	engineagents.Agent
	models     []string
	efforts    []string
	discovered bool
}

func (f fakeSelectionAgent) ID() string       { return "fake-provider" }
func (f fakeSelectionAgent) Models() []string { return f.models }

func (f fakeSelectionAgent) Efforts(string) []string { return f.efforts }

func (f fakeSelectionAgent) Capabilities() engineagents.Capabilities {
	return engineagents.Capabilities{ModelDiscovery: f.discovered}
}

type stubAgentsForSelection struct {
	engineagents.Agents
	agent engineagents.Agent
}

func (s stubAgentsForSelection) Get(
	context.Context, string, string,
) (engineagents.Agent, error) {
	return s.agent, nil
}

func conversationsForSelectionTest(agent engineagents.Agent) *Conversations {
	return &Conversations{
		runnerStore: stubLiveRunnerForSelection{runner: engineagents.Runner{ID: "r1", ProviderID: "fake-provider"}},
		agents:      stubAgentsForSelection{agent: agent},
		home:        func() (string, error) { return "/home", nil },
	}
}

func TestValidateSelection_AcceptsAModelTheResolvedCatalogueDeclares(t *testing.T) {
	c := conversationsForSelectionTest(fakeSelectionAgent{models: []string{"opus"}, efforts: []string{"low"}})

	err := c.validateSelection(context.Background(), "chat-1", "opus", "low")

	require.NoError(t, err)
}

func TestValidateSelection_RejectsAModelAResolvedCatalogueOmits(t *testing.T) {
	c := conversationsForSelectionTest(fakeSelectionAgent{models: []string{"opus"}})

	err := c.validateSelection(context.Background(), "chat-1", "not-a-real-model", "")

	require.Error(t, err)
}

// Required property: an empty/unresolved discovery must never reject a
// non-empty model.
func TestValidateSelection_AnUnresolvedDiscoveryAcceptsAnyModel(t *testing.T) {
	c := conversationsForSelectionTest(fakeSelectionAgent{discovered: true})

	err := c.validateSelection(context.Background(), "chat-1", "gpt-6-astra", "ultra")

	require.NoError(t, err)
}

func TestValidateSelection_AStaticEmptyCatalogueStillRejects(t *testing.T) {
	c := conversationsForSelectionTest(fakeSelectionAgent{discovered: false})

	err := c.validateSelection(context.Background(), "chat-1", "anything", "")

	require.Error(t, err, "no catalogue at all (not discovery-backed) must keep rejecting, same as before")
}
