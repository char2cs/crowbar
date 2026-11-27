package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

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

func TestStartModelDiscoveryWarmup_NilAgentsIsANoOp(t *testing.T) {
	startModelDiscoveryWarmup(context.Background(), &engine.Container{}, "/tmp/crowbar-home")
	startModelDiscoveryWarmup(context.Background(), nil, "/tmp/crowbar-home")
}
