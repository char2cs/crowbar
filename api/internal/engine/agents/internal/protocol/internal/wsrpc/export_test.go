package wsrpc

// WithMaxQueued shrinks the mailbox bound so a test can make it overflow.
func WithMaxQueued(n int) Option {
	return func(c *Conn) { c.mailbox.max = n }
}
