package chat

// WaitQuiescentForTest blocks until the EventStore's asynx instance has
// drained its dispatch queue and run every projection handler (WaitPublish).
// Mutations dispatch via ax.Send (not SendWait), so the store/hub
// projections still run asynchronously after a mutation returns; an external
// test calls this after a mutation to read the store projection with no
// polling and no timeouts.
func WaitQuiescentForTest(
	es EventStore,
) {
	es.(*eventSourced).ax.WaitPublish()
}
