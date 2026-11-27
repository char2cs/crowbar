package chat

func WaitQuiescentForTest(
	es EventStore,
) {
	es.(*eventSourced).ax.WaitPublish()
}
