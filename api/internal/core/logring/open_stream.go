package logring

type openStream struct {
	ring       *ringBuffer
	subscriber *subscriber
	replay     []Record
	seq        uint64
	reset      bool
}

func (s *openStream) Replay() []Record {
	return s.replay
}

func (s *openStream) Seq() uint64 {
	return s.seq
}

func (s *openStream) Reset() bool {
	return s.reset
}

func (s *openStream) Live() <-chan Record {
	return s.subscriber.records
}

func (s *openStream) Close() {
	s.ring.unsubscribe(s.subscriber)
}
