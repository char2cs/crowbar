package logring

type Stream interface {
	Replay() []Record

	Seq() uint64

	Reset() bool

	Live() <-chan Record

	Close()
}
