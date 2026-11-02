package stream

// AwaitReads waits out every snapshot read in flight.
func (h *Hub) AwaitReads() { h.reads.Wait() }
