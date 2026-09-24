package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// fakeWarmupAgents is a minimal engineagents.Agents double whose only
// interesting behaviour is List: it reports the homeDir it was called with
// on listCalled, so a test can block on the ACTUAL warm-up call landing
// rather than guessing with a sleep. startModelDiscoveryWarmup calls List
// alone; every other method here is an unused stub to satisfy the interface.
type fakeWarmupAgents struct {
	listCalled chan string
}

func (f *fakeWarmupAgents) List(_ context.Context, homeDir string) ([]engineagents.Agent, error) {
	f.listCalled <- homeDir
	return nil, nil
}

func (f *fakeWarmupAgents) Get(context.Context, string, string) (engineagents.Agent, error) {
	return nil, nil
}

func (f *fakeWarmupAgents) RecordInjection(string, ...string) {}

func (f *fakeWarmupAgents) ConsumeInjectedPrefix(string, string) (string, bool) {
	return "", false
}

func (f *fakeWarmupAgents) ForgetRunner(string) {}

func (f *fakeWarmupAgents) SetManifestFetchEnabled(func() bool) {}

func (f *fakeWarmupAgents) Close() {}

var _ engineagents.Agents = (*fakeWarmupAgents)(nil)

// TestStartModelDiscoveryWarmup_KicksAgentsListAtBoot pins the actual fix:
// List already forks Refresh/RefreshManifest for every descriptor declaring
// model.discover: or model.manifest: (agents.service.refreshModelsIfDeclared),
// so calling it once at boot — instead of only lazily, on the frontend's own
// first request — is what makes discovery normally resolved before that
// first request ever lands.
func TestStartModelDiscoveryWarmup_KicksAgentsListAtBoot(t *testing.T) {
	fake := &fakeWarmupAgents{listCalled: make(chan string, 1)}
	eng := &engine.Container{Agents: fake}

	startModelDiscoveryWarmup(context.Background(), eng, "/tmp/crowbar-home")

	select {
	case home := <-fake.listCalled:
		assert.Equal(t, "/tmp/crowbar-home", home)
	case <-time.After(2 * time.Second):
		t.Fatal("startModelDiscoveryWarmup never called Agents.List")
	}
}

// TestStartModelDiscoveryWarmup_NilAgentsIsANoOp guards the boot path: a
// Container built without an engine (or an engine with a nil Agents) must
// never panic New().
func TestStartModelDiscoveryWarmup_NilAgentsIsANoOp(t *testing.T) {
	startModelDiscoveryWarmup(context.Background(), &engine.Container{}, "/tmp/crowbar-home")
	startModelDiscoveryWarmup(context.Background(), nil, "/tmp/crowbar-home")
}
