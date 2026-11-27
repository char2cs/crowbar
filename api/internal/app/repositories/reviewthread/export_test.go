package reviewthread

func WaitQuiescentForTest(repo ReviewThread) {
	repo.(*reviewThread).ax.WaitPublish()
}
