package stream

func (h *Hub) AwaitReads() { h.reads.Wait() }
