package agents

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/modeldiscovery"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// discoveringDescriptor mirrors codex.yaml's shape enough to exercise the
// Discover != nil branch of agent.Models/Efforts/DefaultModel — the actual
// probe/mapping is modeldiscovery's own tests; this proves the WIRING from
// its Cache into the Agent surface, without a real provider binary.
func discoveringDescriptor() *spec.Descriptor {
	d := &spec.Descriptor{ID: "probe"}
	d.Model = &spec.ModelSpec{Discover: &spec.ModelDiscoverSpec{
		Command: []string{"debug", "models"}, Adapter: spec.ModelDiscoverAdapterJSON,
	}}
	return d
}

func TestAgent_ModelsAndEffortsReadTheDiscoveryCacheWhenDeclared(t *testing.T) {
	cache := modeldiscovery.NewCache(context.Background())
	cache.Store("probe", []modeldiscovery.Model{
		{ID: "m1", Efforts: []string{"low", "high"}},
		{ID: "m2", Efforts: []string{"medium"}, Default: true},
	})
	a := &agent{spec: discoveringDescriptor(), discovery: cache}

	assert.Equal(t, []string{"m1", "m2"}, a.Models())
	assert.Equal(t, []string{"low", "high"}, a.Efforts("m1"))
	assert.Equal(t, "m2", a.DefaultModel())
}

func TestAgent_ModelsIsEmptyNotErrorWhenDiscoveryHasNotResolved(t *testing.T) {
	a := &agent{spec: discoveringDescriptor(), discovery: modeldiscovery.NewCache(context.Background())}

	assert.Empty(t, a.Models())
	assert.Empty(t, a.Efforts("anything"))
	assert.Empty(t, a.DefaultModel())
}

func TestAgent_ModelsFallsBackToStaticAvailableWhenNoDiscoverBlock(t *testing.T) {
	d := &spec.Descriptor{ID: "static"}
	d.Model = &spec.ModelSpec{Available: []string{"sonnet", "opus"}}
	a := &agent{spec: d, discovery: modeldiscovery.NewCache(context.Background())}

	assert.Equal(t, []string{"sonnet", "opus"}, a.Models())
	assert.Empty(t, a.DefaultModel(), "no discover: block means no source ever states a default")
}

func TestAgent_Capabilities_ModelDiscoveryReflectsTheDiscoverBlock(t *testing.T) {
	discovering := &agent{spec: discoveringDescriptor()}
	assert.True(t, discovering.Capabilities().ModelDiscovery)

	static := &agent{spec: &spec.Descriptor{ID: "static", Model: &spec.ModelSpec{Available: []string{"opus"}}}}
	assert.False(t, static.Capabilities().ModelDiscovery)
}
