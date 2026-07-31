package usecases

// NewAgentToolDepsForTest exposes newAgentToolDeps so the container test can
// assemble the PRODUCTION agent capability surface and assert every tool group is
// actually wired. A tool group whose port is left nil here is not advertised at
// all, and the only visible symptom in a running daemon is an agent that quietly
// has fewer tools than it should — which is why the wiring needs a test of its own
// rather than being trusted to review.
var NewAgentToolDepsForTest = newAgentToolDeps
