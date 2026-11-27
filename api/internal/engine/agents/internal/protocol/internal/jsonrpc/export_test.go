package jsonrpc

func WithMaxQueued(n int) Option {
	return func(c *Conn) { c.mailbox.max = n }
}
