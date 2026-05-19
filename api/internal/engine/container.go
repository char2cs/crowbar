package engine

import "context"

// Container holds stub engine components.
// Orchestrator, FlowRunner, AIBridge, MemoryEngine, DockerPool
// will be implemented in subsequent phases.
type Container struct{}

func New(_ context.Context) (*Container, error) {
	return &Container{}, nil
}
