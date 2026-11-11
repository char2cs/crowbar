package reviewthread

// WaitQuiescentForTest blocks until the repo's asynx instance has drained its
// dispatch queue and run every projection handler (WaitPublish = dispatcher
// WaitIdle + bus WaitForHandlers). It is the deterministic read-your-writes
// barrier: Send returns before the async store/hub projections run (decision 4),
// so an external test calls this after a mutation to read the projection with no
// polling and no timeouts.
func WaitQuiescentForTest(repo ReviewThread) {
	repo.(*reviewThread).ax.WaitPublish()
}
